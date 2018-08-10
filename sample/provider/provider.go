/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provider

import (
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider/helpers"
)

// TODO: apierr "k8s.io/apimachinery/pkg/api/errors"

type externalMetric struct {
	info  provider.ExternalMetricInfo
	value external_metrics.ExternalMetricValue
}

var (
	testingMetrics = []externalMetric{
		{
			info: provider.ExternalMetricInfo{
				Metric: "my-external-metric",
				Labels: map[string]string{
					"foo": "bar",
				},
			},
			value: external_metrics.ExternalMetricValue{
				MetricName: "my-external-metric",
				MetricLabels: map[string]string{
					"foo": "bar",
				},
				Value: *resource.NewQuantity(42, resource.DecimalSI),
			},
		},
		{
			info: provider.ExternalMetricInfo{
				Metric: "my-external-metric",
				Labels: map[string]string{
					"foo": "baz",
				},
			},
			value: external_metrics.ExternalMetricValue{
				MetricName: "my-external-metric",
				MetricLabels: map[string]string{
					"foo": "baz",
				},
				Value: *resource.NewQuantity(43, resource.DecimalSI),
			},
		},
		{
			info: provider.ExternalMetricInfo{
				Metric: "other-external-metric",
				Labels: map[string]string{},
			},
			value: external_metrics.ExternalMetricValue{
				MetricName:   "other-external-metric",
				MetricLabels: map[string]string{},
				Value:        *resource.NewQuantity(44, resource.DecimalSI),
			},
		},
	}
)

type testingProvider struct {
	client dynamic.Interface
	mapper apimeta.RESTMapper

	values          map[provider.CustomMetricInfo]int64
	externalMetrics []externalMetric
}

func NewFakeProvider(client dynamic.Interface, mapper apimeta.RESTMapper) provider.MetricsProvider {
	return &testingProvider{
		client:          client,
		mapper:          mapper,
		values:          make(map[provider.CustomMetricInfo]int64),
		externalMetrics: testingMetrics,
	}
}

func (p *testingProvider) valueFor(groupResource schema.GroupResource, metricName string, namespaced bool) (int64, error) {
	info := provider.CustomMetricInfo{
		GroupResource: groupResource,
		Metric:        metricName,
		Namespaced:    namespaced,
	}

	info, _, err := info.Normalized(p.mapper)
	if err != nil {
		return 0, err
	}

	value := p.values[info]
	value += 1
	p.values[info] = value

	return value, nil
}

func (p *testingProvider) metricFor(value int64, info provider.CustomMetricInfo, namespace string, name string) (*custom_metrics.MetricValue, error) {
	objRef, err := helpers.ReferenceFor(p.mapper, info, namespace, name)
	if err != nil {
		return nil, err
	}

	return &custom_metrics.MetricValue{
		DescribedObject: objRef,
		MetricName:      info.Metric,
		Timestamp:       metav1.Time{time.Now()},
		Value:           *resource.NewMilliQuantity(value*100, resource.DecimalSI),
	}, nil
}

func (p *testingProvider) metricsFor(totalValue int64, info provider.CustomMetricInfo, namespace string, selector labels.Selector) (*custom_metrics.MetricValueList, error) {
	names, err := helpers.ListObjectNames(p.mapper, p.client, info, "", selector)
	if err != nil {
		return nil, err
	}

	res := make([]custom_metrics.MetricValue, len(names))
	for i, name := range names {
		value, err := p.metricFor(100*totalValue/int64(len(res)), info, namespace, name)
		if err != nil {
			return nil, err
		}
		res[i] = *value
	}

	return &custom_metrics.MetricValueList{
		Items: res,
	}, nil
}

func (p *testingProvider) GetRootScopedMetricByName(groupResource schema.GroupResource, name string, metricName string) (*custom_metrics.MetricValue, error) {
	value, err := p.valueFor(groupResource, metricName, false)
	if err != nil {
		return nil, err
	}
	return p.metricFor(value, provider.CustomMetricInfo{
		GroupResource: groupResource,
		Metric:        metricName,
		Namespaced:    false,
	}, "", name)
}

func (p *testingProvider) GetRootScopedMetricBySelector(groupResource schema.GroupResource, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	totalValue, err := p.valueFor(groupResource, metricName, false)
	if err != nil {
		return nil, err
	}

	info := provider.CustomMetricInfo{
		GroupResource: groupResource,
		Metric:        metricName,
		Namespaced:    false,
	}

	return p.metricsFor(totalValue, info, "", selector)
}

func (p *testingProvider) GetNamespacedMetricByName(groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	value, err := p.valueFor(groupResource, metricName, true)
	if err != nil {
		return nil, err
	}
	return p.metricFor(value, provider.CustomMetricInfo{
		GroupResource: groupResource,
		Metric:        metricName,
		Namespaced:    true,
	}, namespace, name)
}

func (p *testingProvider) GetNamespacedMetricBySelector(groupResource schema.GroupResource, namespace string, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	info := provider.CustomMetricInfo{
		GroupResource: groupResource,
		Metric:        metricName,
		Namespaced:    true,
	}

	totalValue, err := p.valueFor(groupResource, metricName, true)
	if err != nil {
		return nil, err
	}

	return p.metricsFor(totalValue, info, namespace, selector)
}

func (p *testingProvider) ListAllMetrics() []provider.CustomMetricInfo {
	// TODO: maybe dynamically generate this?
	return []provider.CustomMetricInfo{
		// these are mostly arbitrary examples
		{
			GroupResource: schema.GroupResource{Group: "", Resource: "pods"},
			Metric:        "packets-per-second",
			Namespaced:    true,
		},
		{
			GroupResource: schema.GroupResource{Group: "", Resource: "services"},
			Metric:        "connections-per-second",
			Namespaced:    true,
		},
		{
			GroupResource: schema.GroupResource{Group: "", Resource: "namespaces"},
			Metric:        "work-queue-length",
			Namespaced:    false,
		},
	}
}
func (p *testingProvider) GetExternalMetric(namespace string, metricName string, metricSelector labels.Selector) (*external_metrics.ExternalMetricValueList, error) {
	matchingMetrics := []external_metrics.ExternalMetricValue{}
	for _, metric := range p.externalMetrics {
		if metric.info.Metric == metricName &&
			metricSelector.Matches(labels.Set(metric.info.Labels)) {
			metricValue := metric.value
			metricValue.Timestamp = metav1.Now()
			matchingMetrics = append(matchingMetrics, metricValue)
		}
	}
	return &external_metrics.ExternalMetricValueList{
		Items: matchingMetrics,
	}, nil
}

func (p *testingProvider) ListAllExternalMetrics() []provider.ExternalMetricInfo {
	externalMetricsInfo := []provider.ExternalMetricInfo{}
	for _, metric := range p.externalMetrics {
		externalMetricsInfo = append(externalMetricsInfo, metric.info)
	}
	return externalMetricsInfo
}
