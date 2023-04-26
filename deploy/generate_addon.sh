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
CLUSTER_ROLE_NAME="harvesterhci.io:cloudprovider"
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

create_clusterrolebinding() {
  echo -e "\\nCreating a clusterrolebinding ${ROLE_BINDING_NAME}"
  kubectl create clusterrolebinding ${ROLE_BINDING_NAME} --serviceaccount=${NAMESPACE}:${SERVICE_ACCOUNT_NAME} --clusterrole=${CLUSTER_ROLE_NAME} --dry-run -o yaml | kubectl apply -f -
}

get_secret_name_from_service_account() {
  read -r SERVER_MAJOR_VERSION SERVER_MINOR_VERSION < <(kubectl version -ojson | jq -r '[.serverVersion | .major, .minor] | join(" ")')
  if [ "${SERVER_MAJOR_VERSION}" -ge 1 ] && [ "${SERVER_MINOR_VERSION}" -ge 24 ]; then
    SECRET_NAME="${SERVICE_ACCOUNT_NAME}-token"
    echo -e "\\nGetting uid of service account ${SERVICE_ACCOUNT_NAME} on ${NAMESPACE}"
    SERVICE_ACCOUNT_UID=$(kubectl get sa "${SERVICE_ACCOUNT_NAME}" --namespace "${NAMESPACE}" -o jsonpath="{.metadata.uid}")
    echo "Service Account uid: ${SERVICE_ACCOUNT_UID}"
    echo -e "\\nCreating a user token secret in ${NAMESPACE} namespace: ${SECRET_NAME}"
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  annotations:
    kubernetes.io/service-account.name: ${SERVICE_ACCOUNT_NAME}
    kubernetes.io/service-account.uid: ${SERVICE_ACCOUNT_UID}
  name: ${SECRET_NAME}
  namespace: ${NAMESPACE}
  ownerReferences:
  - apiVersion: v1
    kind: ServiceAccount
    name: ${SERVICE_ACCOUNT_NAME}
    uid: ${SERVICE_ACCOUNT_UID}
type: kubernetes.io/service-account-token
EOF
  else
    while [ -z "${SECRET_NAME}" ]; do
    echo -e "\\nGetting secret of service account ${SERVICE_ACCOUNT_NAME} on ${NAMESPACE}"
    SECRET_NAME=$(kubectl get sa "${SERVICE_ACCOUNT_NAME}" --namespace="${NAMESPACE}" -o jsonpath="{.secrets[].name}")
    done
  fi
  echo "Secret name: ${SECRET_NAME}"
}

extract_ca_crt_from_secret() {
  while [ -z "${CA_CRT}" ]; do
    echo -e -n "\\nExtracting ca.crt from secret..."
    CA_CRT=$(kubectl get secret --namespace "${NAMESPACE}" "${SECRET_NAME}" -o jsonpath="{.data.ca\.crt}")
  done
  echo "${CA_CRT}" | base64 -d >"${TARGET_FOLDER}/ca.crt"
  printf "done"
}

get_user_token_from_secret() {
  while [ -z "${USER_TOKEN}" ]; do
    echo -e -n "\\nGetting user token from secret..."
    USER_TOKEN=$(kubectl get secret --namespace "${NAMESPACE}" "${SECRET_NAME}" -o jsonpath="{.data.token}" | base64 -d)
  done
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
create_clusterrolebinding
get_secret_name_from_service_account
extract_ca_crt_from_secret
get_user_token_from_secret
set_kube_config_values
echo "########## cloud config ############"
cat ${KUBECFG_FILE_NAME}
echo
echo "########## cloud-init user data ############"
if [[ $OSTYPE == 'darwin'* ]]; then
  KUBECONFIG_B64=$(base64 -b 0 < "${KUBECFG_FILE_NAME}")
else
  KUBECONFIG_B64=$(base64 -w 0 < "${KUBECFG_FILE_NAME}")
fi
cat <<EOF
write_files:
- encoding: b64
  content: ${KUBECONFIG_B64}
  owner: root:root
  path: /etc/kubernetes/cloud-config
  permissions: '0644'
EOF
rm -r ${TARGET_FOLDER}
