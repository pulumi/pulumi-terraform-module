// Copyright 2016-2025, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"os"

	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"

	"github.com/pulumi/pulumi-terraform-module/pkg/modprovider"
)

func main() {
	disableTFLogging()
	err := provider.Main(modprovider.Name(), modprovider.StartServer)
	if err != nil {
		cmdutil.ExitError(err.Error())
	}
}

func disableTFLogging() {
	// Did not find a less intrusive way to disable logging from the auxprovider hosted in-process.
	os.Setenv("TF_LOG_PROVIDER", "off")
	os.Setenv("TF_LOG_SDK", "off")
	os.Setenv("TF_LOG_SDK_PROTO", "off")
}
