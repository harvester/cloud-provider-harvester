#!/bin/bash
set -e
set -o pipefail

# Add user to k8s using service account, no RBAC (must create RBAC after this script)
if [[ -z "$1" ]] || [[ -z "$2" ]]; then
  echo "description: The script is used to add a serviceAccount in the Harvester cluster for the Harvester cloud provider and generate the RKE addon configuration. It depends on kubectl."
  echo "usage: $0 <service_account_name> <namespace>"
  exit 1
fi

SERVICE_ACCOUNT_NAME=$1
ROLE_BINDING_NAME=$1
NAMESPACE="$2"
ROLE_NAME="harvester-cloud-provider"
KUBECFG_FILE_NAME="./tmp/kube/k8s-${SERVICE_ACCOUNT_NAME}-${NAMESPACE}-conf"
TARGET_FOLDER="./tmp/kube"

create_target_folder() {
  echo -n "Creating target directory to hold files in ${TARGET_FOLDER}..."
  mkdir -p "${TARGET_FOLDER}"
  printf "done"
}

create_service_account() {
  echo -e "\\nCreating a service account in ${NAMESPACE} namespace: ${SERVICE_ACCOUNT_NAME}"
  # use kubectl apply to ignore AlreadyExists error
  kubectl create sa "${SERVICE_ACCOUNT_NAME}" --namespace "${NAMESPACE}" --dry-run -o yaml | kubectl apply -f -
}

create_role() {
  echo -e "\\nCreating a role in ${NAMESPACE} namespace: ${ROLE_NAME}"
  echo "
  apiVersion: rbac.authorization.k8s.io/v1
  kind: Role
  metadata:
    name: harvester-cloud-provider
    namespace: ${NAMESPACE}
  rules:
    - apiGroups: [ 'loadbalancer.harvesterhci.io' ]
      resources: [ 'loadbalancers' ]
      verbs: [ 'get', 'watch', 'list', 'update', 'create', 'delete' ]
    - apiGroups: [ 'kubevirt.io' ]
      resources: [ 'virtualmachineinstances' ]
      verbs: [ 'get', 'watch', 'list' ]
    - apiGroups: [ '' ]
      resources: [ 'nodes' ]
      verbs: [ 'get', 'watch', 'list' ]
  " | kubectl apply -f -
}

create_rolebinding() {
  echo -e "\\nCreating a rolebinding in ${NAMESPACE} namespace: ${ROLE_BINDING_NAME}"
  kubectl create rolebinding ${ROLE_BINDING_NAME} --serviceaccount=${NAMESPACE}:${SERVICE_ACCOUNT_NAME} --role=${ROLE_NAME} --dry-run -o yaml | kubectl apply -f -
}

get_secret_name_from_service_account() {
  echo -e "\\nGetting secret of service account ${SERVICE_ACCOUNT_NAME} on ${NAMESPACE}"
  SECRET_NAME=$(kubectl get sa "${SERVICE_ACCOUNT_NAME}" --namespace="${NAMESPACE}" -o jsonpath="{.secrets[].name}")
  echo "Secret name: ${SECRET_NAME}"
}

extract_ca_crt_from_secret() {
  echo -e -n "\\nExtracting ca.crt from secret..."
  kubectl get secret --namespace "${NAMESPACE}" "${SECRET_NAME}" -o jsonpath="{.data.ca\.crt}" | base64 -d >"${TARGET_FOLDER}/ca.crt"
  printf "done"
}

get_user_token_from_secret() {
  echo -e -n "\\nGetting user token from secret..."
  USER_TOKEN=$(kubectl get secret --namespace "${NAMESPACE}" "${SECRET_NAME}" -o jsonpath="{.data.token}" | base64 -d)
  printf "done"
}

set_kube_config_values() {
  context=$(kubectl config current-context)
  echo -e "\\nSetting current context to: $context"

  CLUSTER_NAME=$(kubectl config get-contexts "$context" | awk '{print $3}' | tail -n 1)
  echo "Cluster name: ${CLUSTER_NAME}"

  ENDPOINT=$(kubectl config view \
    -o jsonpath="{.clusters[?(@.name == \"${CLUSTER_NAME}\")].cluster.server}")
  echo "Endpoint: ${ENDPOINT}"

  # Set up the config
  echo -e "\\nPreparing k8s-${SERVICE_ACCOUNT_NAME}-${NAMESPACE}-conf"
  echo -n "Setting a cluster entry in kubeconfig..."
  kubectl config set-cluster "${CLUSTER_NAME}" \
    --kubeconfig="${KUBECFG_FILE_NAME}" \
    --server="${ENDPOINT}" \
    --certificate-authority="${TARGET_FOLDER}/ca.crt" \
    --embed-certs=true

  echo -n "Setting token credentials entry in kubeconfig..."
  kubectl config set-credentials \
    "${SERVICE_ACCOUNT_NAME}-${NAMESPACE}-${CLUSTER_NAME}" \
    --kubeconfig="${KUBECFG_FILE_NAME}" \
    --token="${USER_TOKEN}"

  echo -n "Setting a context entry in kubeconfig..."
  kubectl config set-context \
    "${SERVICE_ACCOUNT_NAME}-${NAMESPACE}-${CLUSTER_NAME}" \
    --kubeconfig="${KUBECFG_FILE_NAME}" \
    --cluster="${CLUSTER_NAME}" \
    --user="${SERVICE_ACCOUNT_NAME}-${NAMESPACE}-${CLUSTER_NAME}" \
    --namespace="${NAMESPACE}"

  echo -n "Setting the current-context in the kubeconfig file..."
  kubectl config use-context "${SERVICE_ACCOUNT_NAME}-${NAMESPACE}-${CLUSTER_NAME}" \
    --kubeconfig="${KUBECFG_FILE_NAME}"
}

create_target_folder
create_service_account
create_role
create_rolebinding
get_secret_name_from_service_account
extract_ca_crt_from_secret
get_user_token_from_secret
set_kube_config_values
echo "########## cloud config ############"
cat ${KUBECFG_FILE_NAME}
rm -r ${TARGET_FOLDER}
