# Secret Manager
Secret Manager is a tool for managing secrets in IBM Cloud's secret manager service. It provides a command line interface for rotating them to ensure their security.

## Features

- rotate: Rotates the secrets from the secret manager

## Installation

```bash
$ go get -u sigs.k8s.io/provider-ibmcloud-test-infra/secret-manager
```

## How to run

```
$ export IBMCLOUD_ENV_FILE=/<path-to-apikey>/ibm-api-key
$ secret-manager rotate --instance-id <secret-manager-instance-id> --labels rotate:true
```