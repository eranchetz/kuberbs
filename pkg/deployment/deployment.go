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
	"github.com/Sirupsen/logrus"
	"github.com/doitintl/kuberbs/pkg/metrics"
	"github.com/doitintl/kuberbs/pkg/utils"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Container struct {
	Name  string
	Image string
}

type Deployment struct {
	Name      string
	NameSpace string
	client    kubernetes.Interface
	current   *apps_v1.Deployment
	new       *apps_v1.Deployment
}

func NewDeploymentController(kubeClient kubernetes.Interface, current *apps_v1.Deployment, new *apps_v1.Deployment) *Deployment {

	return &Deployment{
		Name:      current.Name,
		NameSpace: current.Namespace,
		current:   current,
		new:       new,
		client:    kubeClient,
	}
}

func (d *Deployment) SaveCurrentDeploymentState() {
	if d.new != nil {
		for _, v := range d.current.Spec.Template.Spec.Containers {
			meta_v1.SetMetaDataAnnotation(&d.new.ObjectMeta, "kuberbs.pod."+v.Name, v.Image)
		}
		_, err := d.client.AppsV1().Deployments(d.new.Namespace).Update(d.new)
		if err != nil {
			logrus.Error(err)
		}
	} else {
		logrus.Debug("No new deployment object found")
	}
}

func (d *Deployment) StartWatch(m metrics.Metrics) {
	logrus.Debugf("Calling m.Run for %s @ %s", d.Name, d.NameSpace)
	m.Run()
}


func (d *Deployment) ShouldWatch() bool {
	commonContainers := d.ContainersIntersection()
	if len (commonContainers) == 0 {
		return false
	}
	return true
}

func (d *Deployment) ContainersIntersection() (c []core_v1.Container) {
	for _, item := range d.new.Spec.Template.Spec.Containers {
		if d.containsAndChanged(d.current.Spec.Template.Spec.Containers, item) {
			c = append(c, item)
		}

	}
	return
}

func (d *Deployment) MetricsHandler(rate float64) {

}
func (d *Deployment) containsAndChanged(a []core_v1.Container, b core_v1.Container) bool {
	for _, item := range a {
		if (item.Name == b.Name) && (utils.IsSameImage(item, b)) {
			return true
		}
	}
	return false
}

/*
func (d *Deployment) Get()  {
	dd, err := d.client.AppsV1().Deployments(d.NameSpace).Get(d.Name, meta_v1.GetOptions{})
	if err != nil {
		logrus.Error(err)
		//return nil
	}
	data := make(map[string]string)
	for _, v := range dd.Status.Conditions {
		if v.Status == "Progressing" {
		//	d.LastUpdated = v.LastUpdateTime
		}
	}

	data[d.NameSpace+"-"+d.Name+"-LastUpdateTime"] = d.LastUpdated.String()
	for _, v := range dd.Spec.Template.Spec.Containers {
	//	d.Containers = append(d.Containers, Container{
	//		Name:  v.Name,
	//		Image: v.Image,
		})
		data[d.NameSpace+"-"+d.Name+"-Container-"+v.Name+"-Image"] = v.Image
	}
	//return data
}

func (d *Deployment) Save() {
	cm := GetInstance(d.client)
	data := d.Get()
	cm.UpdateKeys(data)
	cm.Save()

}
*/
