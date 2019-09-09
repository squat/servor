# Servor

Servor is a tiny HTTP API that can control a servo connected to a GPIO pin.

[![Build Status](https://travis-ci.org/squat/servor.svg?branch=master)](https://travis-ci.org/squat/servor)
[![Go Report Card](https://goreportcard.com/badge/github.com/squat/servor)](https://goreportcard.com/report/github.com/squat/servor)

## Getting Started

The easiest way to use servor is with the prebuilt container, e.g.:

```shell
$ docker run -p 8080:8080 squat/servor --pin=18
```

This container can be deployed to a Kubernetes cluster running on nodes with GPIO, e.g. a Raspberry PI running k3s:

```shell
kubectl apply -f https://raw.githubusercontent.com/squat/servor/master/manifests/servor.yaml
kubectl port-forward svc/servor 8080
```

Once running, open the servor UI in a browser and use the arrow keys or buttons to control the connected servo:

```shell
$BROWSER http://localhost:8080
```

## API

Servor exposes two API endpoints:

### POST `/api/left`
This endpoint moves the servo one step to the left.

### POST `/api/right`
This endpoint moves the servo one step to the right.
