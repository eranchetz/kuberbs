// Copyright © 2018 Aviv Laufer <aviv.laufer@gmail.com>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package controller

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/derekparker/delve/pkg/config"
	"github.com/doitintl/kuberbs/pkg/api/types/v1"
	client_v1 "github.com/doitintl/kuberbs/pkg/clientset/v1"
	"github.com/doitintl/kuberbs/pkg/deployment"
	"github.com/doitintl/kuberbs/pkg/metrics"
	st "github.com/doitintl/kuberbs/pkg/metrics/stackdriver"
	"github.com/doitintl/kuberbs/pkg/utils"
	apps_v1 "k8s.io/api/apps/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const maxRetries = 5

var serverStartTime time.Time

// Event indicate the informerEvent
type Event struct {
	key          string
	eventType    string
	namespace    string
	resourceType string
	old          *apps_v1.Deployment
	new          *apps_v1.Deployment
}

// Controller object
type Controller struct {
	logger     *logrus.Entry
	client     kubernetes.Interface
	clinet_set *client_v1.V1Client
	queue      workqueue.RateLimitingInterface
	informer   cache.SharedIndexInformer
}

func Start(conf *config.Config) {
	var kubeClient kubernetes.Interface
	_, err := rest.InClusterConfig()
	if err != nil {
		logrus.Fatal(err)
	} else {
		kubeClient = utils.GetClient()
	}
	//start deployments watcher
	deploymentsInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.AppsV1().Deployments(meta_v1.NamespaceAll).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.AppsV1().Deployments(meta_v1.NamespaceAll).Watch(options)
			},
		},
		&apps_v1.Deployment{},
		0, //Skip resync
		cache.Indexers{},
	)

	cd := newResourceController(kubeClient, deploymentsInformer, "deployment")
	deploymentsStopCh := make(chan struct{})
	defer close(deploymentsStopCh)

	go cd.Run(deploymentsStopCh)

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)
	signal.Notify(sigterm, syscall.SIGINT)
	<-sigterm
}

func newResourceController(client kubernetes.Interface, informer cache.SharedIndexInformer, resourceType string) *Controller {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	var newEvent Event
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			return
		},
		UpdateFunc: func(old, new interface{}) {

			/*dd := new.(*apps_v1beta1.Deployment)
			for b, a := range dd.ObjectMeta.Annotations {
				if strings.HasPrefix(b, "kuberbs.") {
					logrus.Debug(b, a)
				}
			}*/

			var err error
			newDeployment := new.(*apps_v1.Deployment)
			if utils.DeploymentComplete(newDeployment, &newDeployment.Status) {
				newEvent.key, err = cache.MetaNamespaceKeyFunc(old)
				newEvent.eventType = "update"
				newEvent.resourceType = resourceType
				newEvent.old = old.(*apps_v1.Deployment)
				newEvent.new = newDeployment
				logrus.WithField("pkg", "kuberbs-"+resourceType).Debugf("Processing update to %v: %s", resourceType, newEvent.key)
				if err == nil {
					queue.Add(newEvent)
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			return
		},
	})
	var cfg *rest.Config
	cfg, err := rest.InClusterConfig()
	if err != nil {
		panic(err)
	}
	v1.AddToScheme(scheme.Scheme)
	clientSet, err := client_v1.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}

	return &Controller{
		logger:     logrus.WithField("pkg", "kuberbs-"+resourceType),
		client:     client,
		informer:   informer,
		queue:      queue,
		clinet_set: clientSet,
	}
}

// Run starts the kuberbs controller
func (c *Controller) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.logger.Info("Starting kuberbs controller")
	serverStartTime = time.Now().Local()

	go c.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	c.logger.Info("kuberbs controller synced and ready")

	wait.Until(c.runWorker, time.Second, stopCh)
}

// HasSynced is required for the cache.Controller interface.
func (c *Controller) HasSynced() bool {
	return c.informer.HasSynced()
}

// LastSyncResourceVersion is required for the cache.Controller interface.
func (c *Controller) LastSyncResourceVersion() string {
	return c.informer.LastSyncResourceVersion()
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
		// continue looping
	}
}

func (c *Controller) processNextItem() bool {
	newEvent, quit := c.queue.Get()

	if quit {
		return false
	}
	defer c.queue.Done(newEvent)
	err := c.processItem(newEvent.(Event))
	if err == nil {
		// No error, reset the ratelimit counters
		c.queue.Forget(newEvent)
	} else if c.queue.NumRequeues(newEvent) < maxRetries {
		c.logger.Errorf("Error processing %s (will retry): %v", newEvent.(Event).key, err)
		c.queue.AddRateLimited(newEvent)
	} else {
		// err != nil and too many retries
		c.logger.Errorf("Error processing %s (giving up): %v", newEvent.(Event).key, err)
		c.queue.Forget(newEvent)
		utilruntime.HandleError(err)
	}

	return true
}

func (c *Controller) processItem(newEvent Event) error {
	var metricsSource string
	watchPeriod := 0
	switch newEvent.eventType {
	case "update":
		rbs, err := c.clinet_set.Rbs(client_v1.RbsNameSpace).List(meta_v1.ListOptions{})
		if err != nil {
			panic(err)
		}
		status := false
		for i, ns := range rbs.Items[0].Spec.Namespaces {
			if strings.EqualFold(ns.Name, newEvent.old.ObjectMeta.Namespace) {
				for _, name := range rbs.Items[0].Spec.Namespaces[i].Deployments {
					if strings.EqualFold(name, newEvent.old.ObjectMeta.Name) {
						status = true
						metricsSource = strings.ToLower(rbs.Items[0].Spec.MetricsSource)
						watchPeriod = rbs.Items[0].Spec.WatchPeriod
						logrus.Infof("Found a new deployment to handle namespace %s name %s", newEvent.old.ObjectMeta.Namespace, newEvent.old.ObjectMeta.Name)
						continue
					}
				}
			}
		}
		if status {
			dp := deployment.NewDeploymentController(c.client, newEvent.old, newEvent.new)
			if dp.ShouldWatch() {
				dp.SaveCurrentDeploymentState()
				var m metrics.Metrics
				et := time.Now().Add(time.Duration(watchPeriod) * time.Minute)
				logrus.Debugf("End %v Now %v %v", et, time.Now(), watchPeriod)
				switch metricsSource {
				case "stackdriver":
					m = st.NewStackDriver(time.Now(), et, "logging.googleapis.com/user/hello-kubernetes", dp.MetricsHandler)
				default:
					return nil
				}
				logrus.Infof("Starting to watch deployment %s", dp.Name)
				dp.StartWatch(m)
			}
		}
		return nil
	case "create":

		return nil
	case "delete":
		return nil
	}
	return nil
}
