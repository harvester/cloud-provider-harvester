## Cloud-Config User Guide
The cloud-config of Harvester Cloud Provider is toml formatted. The usage of toml refers to the [official user guide](https://github.com/toml-lang/toml/blob/master/toml.md#user-content-string).
### How to get the server and certificate
We can get the server and certificate-authority-data from the kubeConfig file of the Harvester cluster. The following is part of the kubeConfig file.
```
apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: xxxxxxxxxxxxxx
    server: xxxxxxxxxxx
```

> Note: The `certificate-authority-data` in the kubeConfig is encoded by base64.

### How to get the token
The Harvester cloud provider requires a token from Harvester. This document will introduce how to get the token.

> Note: All steps should be executed in the Harvester cluster.

#### Create a serviceAccount
``` yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ccm-user
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: harvester-cloud-provider
  namespace: default
rules:
  - apiGroups: [ "loadbalancer.harvesterhci.io" ]
    resources: [ "loadbalancers" ]
    verbs: [ "get", "watch", "list", "update", "create", "delete" ]
   - apiGroups: [ "kubevirt.io" ]
     resources: ["virtualmachines"]
     verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: harvester-load-balancer
  namespace: default
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: harvester-cloud-provider
subjects:
  - kind: ServiceAccount
    name: ccm-user
    namespace: default
```

#### Get the token
- Get the details of the ccm-user ServiceAccount and the corresponding secret.

``` shell
➜  ~ kubectl get sa ccm-user -o yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","kind":"ServiceAccount","metadata":{"annotations":{},"name":"ccm-user","namespace":"default"}}
  creationTimestamp: "2021-06-09T10:11:54Z"
    manager: kubectl
    operation: Update
    time: "2021-06-09T10:11:54Z"
  name: ccm-user
  namespace: default
  resourceVersion: "17254240"
  uid: df6a5b9f-4450-4e41-903a-54a8446b18be
secrets:
- name: ccm-user-token-crjbg
```

- Get the token from secret
```
➜  ~ kubectl get secret ccm-user-token-crjbg -o jsonpath='{.data.token}' | base64 -d
```

### An Example
``` toml
[cluster]
name = "rke"
[harvester]
server = "https://172.16.1.233:6443"
certificate-authority-data = '''
-----BEGIN CERTIFICATE-----
MIIBeDCCAR2gAwIBAgIBADAKBggqhkjOPQQDAjAjMSEwHwYDVQQDDBhrM3Mtc2Vy
dmVyLWNhQDE2MjQyNzI0MDcwHhcNMjEwNjIxMTA0NjQ3WhcNMzEwNjE5MTA0NjQ3
WjAjMSEwHwYDVQQDDBhrM3Mtc2VydmVyLWNhQDE2MjQyNzI0MDcwWTATBgcqhkjO
PQIBBggqhkjOPQMBBwNCAATT4mcAZPBntlJLKXJ77vRf9w0VOx2Wu3uu6sY5JKw3
QslHKugnh4fCB7+i5YG9zAvfjDaT/4tOW4N1m2QwW6vFo0IwQDAOBgNVHQ8BAf8E
BAMCAqQwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQUfR+UGg8qGyVInZtieUyt
HCtvYCMwCgYIKoZIzj0EAwIDSQAwRgIhAN10cDUe+cNXoUeWyedFpE/fkxM9oRXk
/n0eWAHHIkEfAiEAwwtQcnd/hrU7NvAwHUh5w+Gdi4e9Hkkobno3eu1xOiA=
-----END CERTIFICATE-----
'''
token = "xxxxxxxxxxxxxxxxxx"
namespace = "ccm"
```
