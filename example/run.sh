#!/bin/sh

go build -o test nest.go test.go
env $(cat ../.env | xargs) ./test
