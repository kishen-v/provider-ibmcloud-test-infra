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

package secretmanager

import (
	"fmt"

	"github.com/IBM/go-sdk-core/v5/core"
	sm "github.com/IBM/secrets-manager-go-sdk/v2/secretsmanagerv2"
)

type Interface interface {
	// GetIamSecrets retrieves the secrets of type iam_credentials from the IBM Cloud Secret Manager service.
	GetIamSecrets([]string) ([]sm.SecretMetadataIntf, error)
	// RotateSecret rotates the secret with the given id of type "iam_credentials".
	RotateSecret(*string) error
}

var _ Interface = &SecretManager{}

type SecretManager struct {
	smv2 *sm.SecretsManagerV2
}

// NewSecretsManager returns a new instance of the SecretManager struct.
func New(auth core.Authenticator, instanceID, region string) (*SecretManager, error) {
	smv2, err := sm.NewSecretsManagerV2(&sm.SecretsManagerV2Options{
		URL:           fmt.Sprintf("https://%s.%s.secrets-manager.appdomain.cloud", instanceID, region),
		Authenticator: auth,
	})
	if err != nil {
		return nil, err
	}

	return &SecretManager{
		smv2: smv2,
	}, nil
}

// GetIamSecrets retrieves the secrets of type iam_credentials from the IBM Cloud Secret Manager service.
func (s *SecretManager) GetIamSecrets(labels []string) ([]sm.SecretMetadataIntf, error) {
	listSecretsOptions := &sm.ListSecretsOptions{
		Limit:       core.Int64Ptr(int64(10)),
		SecretTypes: []string{"iam_credentials"},
	}

	if len(labels) > 0 {
		listSecretsOptions.MatchAllLabels = labels
	}

	pager, err := s.smv2.NewSecretsPager(listSecretsOptions)
	if err != nil {
		return nil, err
	}

	return pager.GetAll()
}

// RotateSecret rotates the secret with the given id of type "iam_credentials".
func (s *SecretManager) RotateSecret(id *string) error {
	options := sm.CreateSecretVersionOptions{
		SecretID:               id,
		SecretVersionPrototype: &sm.IAMCredentialsSecretVersionPrototype{},
	}
	_, _, err := s.smv2.CreateSecretVersion(&options)
	if err != nil {
		return err
	}
	return nil
}
