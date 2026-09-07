package main

import (
	"os"

	kubevirtv1 "kubevirt.io/api/core/v1"

	controllergen "github.com/rancher/wrangler/v3/pkg/controller-gen"
	"github.com/rancher/wrangler/v3/pkg/controller-gen/args"
)

func main() {
	os.Unsetenv("GOPATH")
	controllergen.Run(args.Options{
		OutputPackage: "github.com/harvester/harvester-cloud-provider/pkg/generated",
		Boilerplate:   "hack/boilerplate.go.txt",
		Groups: map[string]args.Group{
			kubevirtv1.SchemeGroupVersion.Group: {
				Types: []interface{}{
					kubevirtv1.VirtualMachine{},
					kubevirtv1.VirtualMachineInstance{},
				},
				GenerateTypes:   false,
				GenerateClients: false, // ClientSets are not used yet, don't generate.
			},
		},
	})
}
