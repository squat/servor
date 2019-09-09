# Servor

Servor is a tiny HTTP API that can control a servo connected to a GPIO pin.

[![Build Status](https://travis-ci.org/squat/servor.svg?branch=master)](https://travis-ci.org/squat/servor)
[![Go Report Card](https://goreportcard.com/badge/github.com/squat/servor)](https://goreportcard.com/report/github.com/squat/servor)

## How it works

Servor exposes two API endpoints:

### POST `/api/left`
This endpoint moves the servo one step to the left.

### POST `/api/right`
This endpoint moves the servo one step to the right.
