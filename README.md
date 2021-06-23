Harvester Cloud Provider
==========================
Harvester Cloud Provider implements the Kubernetes Cloud Controller Manager and makes Harvester a Kubernetes cloud provider.

## Manifests and Deploying
Before deploying the Harvester cloud provider, your Kubernetes should be configured to allow external cloud providers.<br>
The ./manifests folder contains useful YAML manifests to use for deploying and developing the Harvester Cloud provider. The simply YAML creates a Deployment using the rancher/harvester-cloud-provider container.<br>
It's recommended to deploy the Harvester cloud provider at the same time when spin up the Kubernetes cluster using the Harvester node driver.<br>
You should be able to config it with the following steps:
- Select the external cloud provider option.

  ![](./doc/image/allow-cloud-provider.png)

- Edit the RKE YAML to add custom plugins. 
  ```
  rancher_kubernetes_engine_config:
  ...
    cloud_provider:
      name: external
    addons: |-
      ---
      apiVersion: v1
      kind: Secret
      metadata:
        name: cloud-config
        namespace: kube-system
      type: Opaque
      stringData:
        cloud-config.toml: |
          [cluster]
          name = <cluster name>
          [harvester]
          server = <Harvester cluster api-server url>
          certificate-authority-data = <Harvester cluster CA from kubeconfig>
          token = <Harvester service account token>
          namespace = <Namespace in Harvester where to create resource>
    addons_include:
    - https://raw.githubusercontent.com/harvester/cloud-provider-harvester/master/manifests/rbac.yaml
    - https://raw.githubusercontent.com/harvester/cloud-provider-harvester/master/manifests/deployment.yaml
  ```
  The details about how to configure cloud-config refer to the [user guide](/doc/cloud-config-user-guide.md)

## License
Copyright (c) 2021 Rancher Labs, Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License. You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions and limitations under the License.
