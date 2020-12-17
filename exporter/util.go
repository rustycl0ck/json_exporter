// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package exporter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kawamuray/jsonpath"
	"github.com/prometheus-community/json_exporter/config"
	"github.com/prometheus/client_golang/prometheus"
	pconfig "github.com/prometheus/common/config"
)

func MakeMetricName(parts ...string) string {
	return strings.Join(parts, "_")
}

func SanitizeValue(v *jsonpath.Result) (float64, error) {
	var value float64
	var boolValue bool
	var err error
	switch v.Type {
	case jsonpath.JsonNumber:
		value, err = parseValue(v.Value)
	case jsonpath.JsonString:
		// If it is a string, lets pull off the quotes and attempt to parse it as a number
		value, err = parseValue(v.Value[1 : len(v.Value)-1])
	case jsonpath.JsonNull:
		value = math.NaN()
	case jsonpath.JsonBool:
		if boolValue, err = strconv.ParseBool(string(v.Value)); boolValue {
			value = 1.0
		} else {
			value = 0.0
		}
	default:
		value, err = parseValue(v.Value)
	}
	if err != nil {
		// Should never happen.
		return -1.0, err
	}
	return value, err
}

func parseValue(bytes []byte) (float64, error) {
	value, err := strconv.ParseFloat(string(bytes), 64)
	if err != nil {
		return -1.0, fmt.Errorf("failed to parse value as float; value: %q; err: %w", bytes, err)
	}
	return value, nil
}

func CreateMetricsList(c config.Config) ([]JsonMetric, error) {
	var metrics []JsonMetric
	for _, metric := range c.Metrics {
		switch metric.Type {
		case config.ValueScrape:
			constLabels := make(map[string]string)
			var variableLabels, variableLabelsValues []string
			for k, v := range metric.Labels {
				if len(v) < 1 || v[0] != '$' {
					// Static value
					constLabels[k] = v
				} else {
					variableLabels = append(variableLabels, k)
					variableLabelsValues = append(variableLabelsValues, v)
				}
			}
			jsonMetric := JsonMetric{
				Desc: prometheus.NewDesc(
					metric.Name,
					metric.Help,
					variableLabels,
					constLabels,
				),
				KeyJsonPath:     metric.Path,
				LabelsJsonPaths: variableLabelsValues,
			}
			metrics = append(metrics, jsonMetric)
		case config.ObjectScrape:
			for subName, valuePath := range metric.Values {
				name := MakeMetricName(metric.Name, subName)
				constLabels := make(map[string]string)
				var variableLabels, variableLabelsValues []string
				for k, v := range metric.Labels {
					if len(v) < 1 || v[0] != '$' {
						// Static value
						constLabels[k] = v
					} else {
						variableLabels = append(variableLabels, k)
						variableLabelsValues = append(variableLabelsValues, v)
					}
				}
				jsonMetric := JsonMetric{
					Desc: prometheus.NewDesc(
						name,
						metric.Help,
						variableLabels,
						constLabels,
					),
					KeyJsonPath:     metric.Path,
					ValueJsonPath:   valuePath,
					LabelsJsonPaths: variableLabelsValues,
				}
				metrics = append(metrics, jsonMetric)
			}
		default:
			return nil, fmt.Errorf("Unknown metric type: '%s', for metric: '%s'", metric.Type, metric.Name)
		}
	}
	return metrics, nil
}

func FetchJson(ctx context.Context, logger log.Logger, endpoint string, c config.Config, tplValues url.Values) ([]byte, error) {
	var req *http.Request
	httpClientConfig := c.HTTPClientConfig
	client, err := pconfig.NewClientFromConfig(httpClientConfig, "fetch_json", true, false)
	if err != nil {
		level.Error(logger).Log("msg", "Error generating HTTP client", "err", err) //nolint:errcheck
		return nil, err
	}

	if c.Body.Content == "" {
		req, err = http.NewRequest("GET", endpoint, nil)
	} else {
		br := strings.NewReader(c.Body.Content)
		if c.Body.Templatize {
			tpl, err := template.New("base").Funcs(sprig.GenericFuncMap()).Parse(c.Body.Content)
			if err != nil {

				level.Error(logger).Log("msg", "Failed to create a new template from body content", "err", err, "content", c.Body.Content) //nolint:errcheck
			}
			var b strings.Builder
			if err := tpl.Execute(&b, tplValues); err != nil {
				level.Error(logger).Log("msg", "Failed to render template with values", "err", err, "tempalte", c.Body.Content, "values", tplValues) //nolint:errcheck
			}
			br = strings.NewReader(b.String())
		}
		req, err = http.NewRequest("POST", endpoint, br)
	}
	req = req.WithContext(ctx)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create request", "err", err) //nolint:errcheck
		return nil, err
	}

	for key, value := range c.Headers {
		req.Header.Add(key, value)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Add("Accept", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		if _, err := io.Copy(ioutil.Discard, resp.Body); err != nil {
			level.Error(logger).Log("msg", "Failed to discard body", "err", err) //nolint:errcheck
		}
		resp.Body.Close()
	}()

	if resp.StatusCode/100 != 2 {
		return nil, errors.New(resp.Status)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}
