package main

import (
	"sigs.k8s.io/kubetest2/pkg/app"

	sigs.k8s.io/provider-ibmcloud-test-infra/kubetest2-tf/deployer""
)

func main() {
	app.Main(deployer.Name, deployer.New)
}
