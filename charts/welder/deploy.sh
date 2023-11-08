#!/usr/bin/env bash
function exec_cmd() {
  echo "${1}"
  echo "Executing '${2}'..."
  eval "${2}"
}

if [[ -z "$1" ]]; then
  echo "Usage: ./deploy.sh <chart-name> [<namespace>]"
  exit 1
fi

NAMESPACE=default
if [[ -n "$2" ]]; then
  NAMESPACE="${2}"
fi

CHARTNAME="${1}"
export DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export ROOT_DIR="$( cd "$DIR"/../.. && pwd)"

if [[ -n "${DEBUG:-}" ]]; then
  PARAMS="${PARAMS} --debug"
fi

SECRETS_FILE="values.secrets.yaml"

if [[ -n "${PROFILE}" ]]; then
  export VALUES="${VALUES} -f ${ROOT_DIR}/charts/${CHARTNAME}/values.${PROFILE}.yaml"
  export KUBECONFIG="${ROOT_DIR}/charts/secrets/kubeconfig.${PROFILE}.yaml"
  SECRETS_FILE="values.secrets.${PROFILE}.yaml"
fi

if [ ! -f "${DIR}/${SECRETS_FILE}" ]; then
  echo >&2 "WARN: Secrets file does not exist: ${DIR}/${SECRETS_FILE}"
else
  VALUES="${VALUES} -f ${DIR}/${SECRETS_FILE}"
fi


if [[ -z "${VALUES}" ]]; then
  for file in $(ls "${DIR}" | grep -E '^values.+.yaml$'); do
    VALUES="${VALUES} -f ${DIR}/${file}"
  done
fi

export KUBECONFIG
export VALUES

exec_cmd "Deploying $CHARTNAME ..." "$(cat <<EOF
  helm upgrade --atomic --install -n "${NAMESPACE}" ${VALUES} ${CHARTNAME} ${DIR} --timeout ${TIMEOUT:-10m} ${PARAMS:-}
EOF
)"