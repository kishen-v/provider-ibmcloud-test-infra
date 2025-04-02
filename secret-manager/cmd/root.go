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

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	instanceID string
	region     string
)

var rootCmd = &cobra.Command{
	Use:   "secret-manager",
	Short: "A tool to help manage secrets in a IBM Cloud secret manager",
	Long:  `A command line tool for managing secrets in a IBM Cloud secret manager`,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&instanceID, "instance-id", "", "Instance ID of the secret manager")
	rootCmd.PersistentFlags().StringVar(&region, "region", "us-south", "Region of the secret manager")
	rootCmd.MarkPersistentFlagRequired("instance-id")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
