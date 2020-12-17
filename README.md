json_exporter
========================
[![CircleCI](https://circleci.com/gh/prometheus-community/json_exporter.svg?style=svg)](https://circleci.com/gh/prometheus-community/json_exporter)

A [prometheus](https://prometheus.io/) exporter which scrapes remote JSON by JSONPath.

# Build

```sh
make build
```

# Example Usage

```sh
$ cat example/data.json
{
    "counter": 1234,
    "values": [
        {
            "id": "id-A",
            "count": 1,
            "some_boolean": true,
            "state": "ACTIVE"
        },
        {
            "id": "id-B",
            "count": 2,
            "some_boolean": true,
            "state": "INACTIVE"
        },
        {
            "id": "id-C",
            "count": 3,
            "some_boolean": false,
            "state": "ACTIVE"
        }
    ],
    "location": "mars"
}

$ cat examples/config.yml
---
metrics:
- name: example_global_value
  path: $.counter
  help: Example of a top-level global value scrape in the json
  labels:
    environment: beta # static label
    location: $.location          # dynamic label

- name: example_value
  type: object
  help: Example of sub-level value scrapes from a json
  path: $.values[*]?(@.state == "ACTIVE")
  labels:
    environment: beta # static label
    id: $.id          # dynamic label
  values:
    active: 1      # static value
    count: $.count # dynamic value
    boolean: $.some_boolean

headers:
  X-Dummy: my-test-header

$ python -m SimpleHTTPServer 8000 &
Serving HTTP on 0.0.0.0 port 8000 ...

$ ./json_exporter --config.file examples/config.yml &

$ curl "http://localhost:7979/probe?target=http://localhost:8000/examples/data.json" | grep ^example
example_global_value{environment="beta",location="mars"} 1234
example_value_active{environment="beta",id="id-A"} 1
example_value_active{environment="beta",id="id-C"} 1
example_value_boolean{environment="beta",id="id-A"} 1
example_value_boolean{environment="beta",id="id-C"} 0
example_value_count{environment="beta",id="id-A"} 1
example_value_count{environment="beta",id="id-C"} 3

# To test through prometheus:
$ docker run --rm -it -p 9090:9090 -v $PWD/examples/prometheus.yml:/etc/prometheus/prometheus.yml --network host prom/prometheus
```
Then head over to http://localhost:9090/graph?g0.range_input=1h&g0.expr=example_value_active&g0.tab=1 or http://localhost:9090/targets to check the scraped metrics or the targets.

# Exposing metrics through HTTPS

web-config.yml
```
# Minimal TLS configuration example. Additionally, a certificate and a key file
# are needed.
tls_server_config:
  cert_file: server.crt
  key_file: server.key
```
Running
```
$ ./json_exporter --config.file examples/config.yml --web.config=web-config.yml &

$ curl -k "https://localhost:7979/probe?target=http://localhost:8000/examples/data.json" | grep ^example
example_global_value{environment="beta",location="mars"} 1234
example_value_active{environment="beta",id="id-A"} 1
example_value_active{environment="beta",id="id-C"} 1
example_value_boolean{environment="beta",id="id-A"} 1
example_value_boolean{environment="beta",id="id-C"} 0
example_value_count{environment="beta",id="id-A"} 1
example_value_count{environment="beta",id="id-C"} 3
```
For futher information about TLS configuration, please visit: [exporter-toolkit/https](https://github.com/prometheus/exporter-toolkit/blob/v0.1.0/https/README.md)

# Sending body content for HTTP POST

 
If `body` paramater is set in config, it will be sent by the exporter as the body content in the scrape request. The HTTP method will also be set as 'POST' in this case.
```yaml
body:
  content: |
    My static information: {"time_diff": "1m25s", "anotherVar": "some value"}
```

The body content can also be a Go Template (https://golang.org/pkg/text/template). All the functions from the Sprig library (https://masterminds.github.io/sprig/) can be used in the template.
All the query parameters sent by prometheus in the scrape query to the exporter, are available as values while rendering the template.

Example using template functions:
```yaml
body:
  content: |
    {"time_diff": "{{ duration `95` }}","anotherVar": "{{ randInt 12 30 }}"}
  templatize: true
```

Example using template functions with values from the query parameters:
```yaml
body:
  content: |
    {"time_diff": "{{ duration `95` }}","anotherVar": "{{ .myVal | first }}"}
  templatize: true
```
Then `curl "http://exporter:7979/probe?target=http://scrape_target:8080/test/data.json&myVal=something"`, would result in sending the following body as the HTTP POST payload to `http://scrape_target:8080/test/data.json`:
```
{"time_diff": "1m35s","anotherVar": "something"}.
```

# Docker

```console
docker run \
  -v $PWD/examples/config.yml:/config.yml
  quay.io/prometheuscommunity/json-exporter \
    --config.file /config.yml
```

# See Also
- [kawamuray/jsonpath](https://github.com/kawamuray/jsonpath#path-syntax) : For syntax reference of JSONPath.
  Originally forked from nicksardo/jsonpath(now is https://github.com/NodePrime/jsonpath).
