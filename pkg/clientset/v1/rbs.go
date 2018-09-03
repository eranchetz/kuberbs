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

package v1

import (
	"github.com/doitintl/kuberbs/pkg/api/types/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// default for local kube-system for remote
const RbsNameSpace = "kube-system"

type RbsInterface interface {
	List(opts metav1.ListOptions) (*v1.RbsList, error)
	Get(name string, options metav1.GetOptions) (*v1.Rbs, error)
	Create(*v1.Rbs) (*v1.Rbs, error)
	Watch(opts metav1.ListOptions) (watch.Interface, error)
	// ...
}

type rbsClient struct {
	restClient rest.Interface
	ns         string
}

func (c *rbsClient) List(opts metav1.ListOptions) (*v1.RbsList, error) {
	result := v1.RbsList{}
	err := c.restClient.
		Get().
		Namespace(c.ns).
		Resource("kuberbs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(&result)

	return &result, err
}

func (c *rbsClient) Get(name string, opts metav1.GetOptions) (*v1.Rbs, error) {
	result := v1.Rbs{}
	err := c.restClient.
		Get().
		Namespace(c.ns).
		Resource("kuberbs").
		Name(name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(&result)

	return &result, err
}

func (c *rbsClient) Create(project *v1.Rbs) (*v1.Rbs, error) {
	result := v1.Rbs{}
	err := c.restClient.
		Post().
		Namespace(c.ns).
		Resource("projects").
		Body(project).
		Do().
		Into(&result)

	return &result, err
}

func (c *rbsClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.restClient.
		Get().
		Namespace(c.ns).
		Resource("kuberbs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}
