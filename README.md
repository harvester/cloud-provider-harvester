Harvester Cloud Provider
==========================
[![Build Status](https://drone-publish.rancher.io/api/badges/harvester/cloud-provider-harvester/status.svg)](https://drone-publish.rancher.io/harvester/cloud-provider-harvester)
[![Go Report Card](https://goreportcard.com/badge/github.com/harvester/cloud-provider-harvester)](https://goreportcard.com/report/github.com/harvester/cloud-provider-harvester)
[![Releases](https://img.shields.io/github/release/harvester/cloud-provider-harvester/all.svg)](https://github.com/harvester/cloud-provider-harvester/releases)

Harvester Cloud Provider implements the Kubernetes Cloud Controller Manager and makes Harvester a Kubernetes cloud provider.

## Manifests and Deploying
Before deploying the Harvester cloud provider, your Kubernetes should be configured to allow external cloud providers.<br>
The ./manifests folder contains useful YAML manifests to use for deploying and developing the Harvester Cloud provider. The simply YAML creates a Deployment using the rancher/harvester-cloud-provider container.<br>
It's recommended to deploy the Harvester cloud provider at the same time when spin up the Kubernetes cluster using the Harvester node driver.<br>

### For RKE:
- Select the external cloud provider option.

  ![](./doc/image/allow-cloud-provider.png)

- Generate addon configuration and add it in the rke yaml.
  ```
  # depend on kubectl to operate the Harvester
  ./deploy/generate_kubeconfig.sh <serviceaccount name> <namespace>
  ```

### Helm chart
To find the helm chart in the [harvester helm chart repo](https://charts.harvesterhci.io).

## License
Copyright (c) 2021 Rancher Labs, Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License. You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions and limitations under the License.
