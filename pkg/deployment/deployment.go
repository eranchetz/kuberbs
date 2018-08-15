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

package deployment

import (
	"container/list"
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/doitintl/kuberbs/pkg/metrics"
	"github.com/doitintl/kuberbs/pkg/utils"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// WatchList contain all active deployments
var WatchList = list.New()

// Deployment holds deployment info & status
type Deployment struct {
	Name      string
	NameSpace string
	Threshold int
	metrics   metrics.Metrics
	Watching  bool
	client    kubernetes.Interface
	current   *apps_v1.Deployment
	new       *apps_v1.Deployment
}

// NewDeploymentController - crteate a New DeploymentController
func NewDeploymentController(kubeClient kubernetes.Interface, current *apps_v1.Deployment, new *apps_v1.Deployment, threshold int) *Deployment {

	d := &Deployment{
		Name:      current.Name,
		NameSpace: current.Namespace,
		Threshold: threshold,
		current:   current,
		new:       new,
		client:    kubeClient,
		Watching:  false,
	}
	return d
}

// SaveCurrentDeploymentState - store image version of current deployment as an annotation
func (d *Deployment) SaveCurrentDeploymentState() error {
	if d.new != nil {
		for _, v := range d.current.Spec.Template.Spec.Containers {
			meta_v1.SetMetaDataAnnotation(&d.new.ObjectMeta, "kuberbs.pod."+v.Name, v.Image)
		}
		_, err := d.client.AppsV1().Deployments(d.new.Namespace).Update(d.new)
		if err != nil {
			logrus.Error(err)
			return err
		}
	} else {
		logrus.Debug("No new deployment object found")
		return fmt.Errorf("No new deployment object found, %v ", d.current.Name)
	}
	return nil
}

// StartWatch - start the metrcis watcher
func (d *Deployment) StartWatch(m metrics.Metrics) {
	logrus.Debugf("Calling m.Run for %s @ %s", d.Name, d.NameSpace)
	d.Watching = true
	d.metrics = m
	go m.Start()
}

// DeploymentComplete - return true if deployment id sone
func (d *Deployment) DeploymentComplete(newd *apps_v1.Deployment) bool {
	return utils.DeploymentComplete(d.new, &newd.Status)
}

// StopWatch - stop the metrics watcher
func (d *Deployment) StopWatch(remove bool) {
	d.metrics.Stop(false)
}

// ShouldWatch do we have containers to watch
func (d *Deployment) ShouldWatch() bool {
	commonContainers := d.containersIntersection()
	return len(commonContainers) > 0
}

func (d *Deployment) containersIntersection() (c []core_v1.Container) {
	for _, item := range d.new.Spec.Template.Spec.Containers {
		if d.containsAndChanged(d.current.Spec.Template.Spec.Containers, item) {
			c = append(c, item)
		}

	}
	return
}

//MetricsHandler - callback for the metrics watcher
func (d *Deployment) MetricsHandler(rate float64) {
	if rate >= float64(d.Threshold) {
		logrus.Infof("It's rollback time, error rate is %v per second", rate)
		d.DoRollback()
	}
}

// WatchDoneHandler - called by metrics watcher when it's done
func (d *Deployment) WatchDoneHandler(remove bool) {
	if remove {
		RemoveFromDeploymentWatchList(d)
	}
	d.Watching = false
}
func (d *Deployment) containsAndChanged(a []core_v1.Container, b core_v1.Container) bool {
	for _, item := range a {
		if item.Name == b.Name {
			if utils.IsSameImage(item, b) {
				return false
			}
		}
	}
	return false
}

// DoRollback TODO make it private
func (d *Deployment) DoRollback() {

	for _, v := range d.new.Spec.Template.Spec.Containers {
		if meta_v1.HasAnnotation(d.new.ObjectMeta, "kuberbs.pod."+v.Name) {
			logrus.Debug(d.new.ObjectMeta.Annotations["kuberbs.pod."+v.Name])
		}
	}
	d.StopWatch(true)
}

// GetDeploymentWatchList returns the WatchList
func GetDeploymentWatchList() *list.List {
	return WatchList
}

// RemoveFromDeploymentWatchList - remove a deployment from the WatchList
func RemoveFromDeploymentWatchList(d *Deployment) {
	for e := WatchList.Front(); e != nil; e = e.Next() {
		dp := e.Value.(*Deployment)
		if (dp.Name == d.Name) && (dp.NameSpace == d.NameSpace) {
			WatchList.Remove(e)
		}
	}

}
