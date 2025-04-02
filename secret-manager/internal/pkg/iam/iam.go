/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package iam

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/IBM/go-sdk-core/v5/core"
)

const (
	// File path where API Key is stored
	apiKeyEnv = "IBMCLOUD_ENV_FILE"
)

// Returns an authenticator for IBM Cloud
func GetAuthenticator() (core.Authenticator, error) {
	key, err := getAPIkeyValue()
	if err != nil {
		return nil, fmt.Errorf("API key retrieval failed, err: %w", err)
	}
	auth := &core.IamAuthenticator{
		ApiKey: key,
	}
	return auth, nil
}

func getAPIkeyValue() (string, error) {
	fileName := os.Getenv(apiKeyEnv)
	if fileName == "" {
		return "", errors.New("IBMCLOUD_ENV_FILE environment must be set and cannot be left empty")
	}

	key, err := os.ReadFile(fileName)
	if err != nil {
		return "", err
	}

	return strings.ReplaceAll(string(key), "\n", ""), nil
}
