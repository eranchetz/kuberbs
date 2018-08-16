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
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/doitintl/kuberbs/pkg/metrics"
	"github.com/doitintl/kuberbs/pkg/utils"
	apps_v1 "k8s.io/api/apps/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// WatchList contain all active deployments
var WatchList = list.New()

// Deployment holds deployment info & status
type Deployment struct {
	Name       string
	NameSpace  string
	Threshold  int
	metrics    metrics.Metrics
	Watching   bool
	IsRollback bool
	client     kubernetes.Interface
	current    *apps_v1.Deployment
	new        *apps_v1.Deployment
}

// NewDeploymentController - crteate a New DeploymentController
func NewDeploymentController(kubeClient kubernetes.Interface, current *apps_v1.Deployment, new *apps_v1.Deployment, threshold int) *Deployment {

	d := &Deployment{
		Name:       current.Name,
		NameSpace:  current.Namespace,
		Threshold:  threshold,
		current:    current,
		new:        new,
		client:     kubeClient,
		Watching:   false,
		IsRollback: false,
	}
	return d
}

// SaveCurrentDeploymentState - store image version of current deployment as an annotation
func (d *Deployment) SaveCurrentDeploymentState() error {
	dp, err := d.client.AppsV1().Deployments(d.NameSpace).Get(d.Name, meta_v1.GetOptions{})
	if err != nil {
		logrus.Error(err)
		return err
	}
	d.new = dp
	if d.new != nil {
		for _, v := range d.current.Spec.Template.Spec.Containers {
			meta_v1.SetMetaDataAnnotation(&d.new.ObjectMeta, "kuberbs.pod."+v.Name, v.Image)
		}
		meta_v1.SetMetaDataAnnotation(&d.new.ObjectMeta, "kuberbs.starttime", time.Now().Format("2006-01-02 15:04:05.999999999 -0700 MST"))
		_, err := d.client.AppsV1().Deployments(d.new.Namespace).Update(d.new)
		if err != nil {
			logrus.Error(err)
			return err
		}
	} else {
		logrus.Debug("No NewDp deployment object found")
		return fmt.Errorf("No NewDp deployment object found, %v ", d.current.Name)
	}
	return nil
}

// StartWatch - start the metrcis watcher
func (d *Deployment) StartWatch(m metrics.Metrics) {
	logrus.Debugf("Startg to Watch! Calling m.Run for %s@%s", d.Name, d.NameSpace)
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
	d.metrics.Stop(remove)
}

// ShouldWatch do we have containers to watch
func (d *Deployment) ShouldWatch() bool {
	return utils.ShouldWatch(d.current, d.new)
}

//MetricsHandler - callback for the metrics watcher
func (d *Deployment) MetricsHandler(rate float64) {
	if rate >= float64(d.Threshold) {
		logrus.WithFields(logrus.Fields{"pkg": "kuberbs", "error-rate": rate}).Info("It's rollback time")
		err := d.doRollback()
		if err != nil {
			logrus.Errorf("Rollback failed", err)
		}
	}
}

// WatchDoneHandler - called by metrics watcher when it's done
func (d *Deployment) WatchDoneHandler(remove bool) error {
	dp, err := d.client.AppsV1().Deployments(d.NameSpace).Get(d.Name, meta_v1.GetOptions{})
	if err != nil {
		logrus.Error(err)
		return err
	}
	meta_v1.SetMetaDataAnnotation(&dp.ObjectMeta, "kuberbs.starttime", "")
	_, err = d.client.AppsV1().Deployments(d.new.Namespace).Update(dp)
	if err != nil {
		logrus.Error(err)
		return err
	}
	if remove {
		RemoveFromDeploymentWatchList(d)
	}
	d.Watching = false
	return nil
}

//IsObservedGenerationSame - any change between deployments?
func (d *Deployment) IsObservedGenerationSame() bool {
	return d.new.Status.ObservedGeneration == d.current.Status.ObservedGeneration
}

// doRollback - do the actual rollback
func (d *Deployment) doRollback() error {
	d.IsRollback = true
	logrus.WithFields(logrus.Fields{"pkg": "kuberbs", "namespace": d.NameSpace, "name": d.Name}).Infof("Starting rollback")
	dp, err := d.client.AppsV1().Deployments(d.NameSpace).Get(d.Name, meta_v1.GetOptions{})
	if err != nil {
		logrus.Error(err)
		return err
	}
	for _, v := range d.new.Spec.Template.Spec.Containers {
		if meta_v1.HasAnnotation(d.new.ObjectMeta, "kuberbs.pod."+v.Name) {
			for i, item := range dp.Spec.Template.Spec.Containers {
				if item.Name == v.Name {
					logrus.Debugf("Reverting from %s to %s", dp.Spec.Template.Spec.Containers[i].Image, d.new.ObjectMeta.Annotations["kuberbs.pod."+v.Name])
					dp.Spec.Template.Spec.Containers[i].Image = d.new.ObjectMeta.Annotations["kuberbs.pod."+v.Name]
				}
			}

		}
	}
	_, err = d.client.AppsV1().Deployments(d.new.Namespace).Update(dp)
	if err != nil {
		logrus.Error(err)
		return err
	}
	d.StopWatch(true)
	logrus.WithFields(logrus.Fields{"pkg": "kuberbs", "namespace": d.NameSpace, "name": d.Name}).Infof("Rollback done")
	return nil
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
