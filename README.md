# kubetop

View Kubernetes nodes, pods, services, and deployments in a glance.

![Screenshot](https://raw.githubusercontent.com/siadat/kubetop/screenshot/screenshot.png)

## Install

* Clone this repo.
* To install dependencies, [install Glide](https://glide.sh), then do `glide install`.
* `go install .`

## Usage

    kubetop [-namespace NAMESPACE]

This command loads Kubernetes configs from the directory specified by `$KUBECONFIG` environment variable.
Otherwise it defaults to `$HOME/.kube/config`.
