#!/bin/bash

# Copyright 2025 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

USER=${USER:-"k8s-prow-job"}
RESOURCE_TYPE=${RESOURCE_TYPE:-"powervs-k8s-conformance"}

trap cleanup EXIT

release_account(){
    url="http://${BOSKOS_HOST}/release?name=${BOSKOS_RESOURCE_NAME}&dest=dirty&owner=${USER}"
    status_code=$(curl -w '%{http_code}' -X POST ${url})

    if [[ ${status_code} != 200 ]]; then
        echo "Failed to release resource: ${BOSKOS_RESOURCE_NAME}"
        exit 1
    fi 
}

checkout_account(){
    resource_type=$1
    set +o xtrace
    url="http://${BOSKOS_HOST}/acquire?type=${resource_type}&state=free&dest=busy&owner=${USER}"
    output=$(curl -X POST ${url})
    [ $? = 0 ] && status_code=200

    if [[ ${status_code} == 200 && ${output} =~ "failed" ]]; then
        echo "Failed to acquire resource from Boskos of type ${RESOURCE_TYPE}"
        exit 1
    elif [[ ${status_code} == 200 ]]; then
        export BOSKOS_RESOURCE_NAME=$(echo ${output} | jq -r '.name')
        export BOSKOS_REGION=$(echo ${output} | jq -r '.userdata["region"]')
        export BOSKOS_RESOURCE_ID=$(echo ${output} | jq -r '.userdata["service-instance-id"]')
        export BOSKOS_ZONE=$(echo ${output} | jq -r '.userdata["zone"]')
    else
        echo "Failed to acquire resource due to invalid response, status code : ${status_code}"
        exit 1
    fi
}

heartbeat_account(){
    count=0
    url="http://${BOSKOS_HOST}/update?name=${BOSKOS_RESOURCE_NAME}&state=busy&owner=${USER}"
    while [ ${count} -lt 120 ]
    do
        status_code=$(curl -s -o /dev/null -w '%{http_code}' -X POST ${url})
        if [[ ${status_code} != 200 ]]; then
            echo "Heart beat to resource '${BOSKOS_RESOURCE_NAME}' failed due to invalid response, status code: ${status_code}"
            exit 1
        fi
        count=$(( $count + 1 ))
        sleep 60
    done
}

cleanup() {
    HEART_BEAT_PID=${1:-}
    # stop the boskos heartbeat
    [[ -z ${HEART_BEAT_PID:-} ]] || kill -9 "${HEART_BEAT_PID}" || true
}

if [ -z "${BOSKOS_HOST:-}" ]; then
    echo "Boskos host is not set. Skipping checkout."
    exit 0
fi

set +o errexit
# Create a temporary file to store environment variables
account_env_var_file="$(mktemp)"

# Checkout account from Boskos and capture output
checkout_account "${RESOURCE_TYPE}" 1> "${account_env_var_file}"
checkout_account_status=$?

# If checkout is successful, source the environment variables
if [ "$checkout_account_status" -eq 0 ]; then
    source "${account_env_var_file}"
else
    echo "Error getting account from Boskos" 1>&2
    rm -f "${account_env_var_file}"
    exit "$checkout_account_status"
fi

# Clean up the temporary file
rm -f "${account_env_var_file}"

# Start heartbeat process
heartbeat_account >> "$ARTIFACTS/boskos.log" 2>&1 &
HEART_BEAT_PID=$!
export HEART_BEAT_PID
