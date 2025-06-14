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

package tests

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/providertest/pulumitest/opttest"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
)

func Test_RdsExample(t *testing.T) {
	t.Parallel()

	tw := newTestWriter(t)

	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)
	// Module written to support the test.
	testProgram, err := filepath.Abs(filepath.Join("../", "examples", "aws-rds-example"))
	require.NoError(t, err)
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))
	integrationTest := newPulumiTest(t, testProgram, localPath)

	// Get a prefix for resource names
	prefix := generateTestResourcePrefix()

	// Set prefix via config
	integrationTest.SetConfig(t, "prefix", prefix)

	// Generate package
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/vpc/aws", "5.19.0", "vpcmod")
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/rds/aws", "6.10.0", "rdsmod")

	integrationTest.Up(t, optup.Diff(),
		optup.ErrorProgressStreams(tw),
		optup.ProgressStreams(tw),
	)

	// Due to some issues in the RDS resource there is going to be drift even after initial creation, which
	// will show up as changes planned in the preview. so we refresh first before preview.
	integrationTest.Preview(t,
		optpreview.Diff(),
		optpreview.ExpectNoChanges(),
		optpreview.ErrorProgressStreams(tw),
		optpreview.ProgressStreams(tw),
	)
}

func Test_EksExample(t *testing.T) {
	t.Parallel()
	localProviderBinPath := ensureCompiledProvider(t)
	tw := newTestWriter(t)
	skipLocalRunsWithoutCreds(t)

	// Module written to support the test.
	testProgram, err := filepath.Abs(filepath.Join("../", "examples", "aws-eks-example"))
	require.NoError(t, err)
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))
	integrationTest := newPulumiTest(t, testProgram, localPath)

	// Get a prefix for resource names
	prefix := generateTestResourcePrefix()

	// Set prefix via config
	integrationTest.SetConfig(t, "prefix", prefix)

	// Generate package
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/vpc/aws", "5.19.0", "vpcmod")
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/eks/aws", "20.34.0", "eksmod")

	integrationTest.Up(t, optup.Diff(),
		optup.ErrorProgressStreams(tw),
		optup.ProgressStreams(tw),
	)

	integrationTest.Preview(t, optpreview.Diff(), optpreview.ExpectNoChanges(),
		optpreview.ErrorProgressStreams(tw),
		optpreview.ProgressStreams(tw),
	)
}

func Test_AlbExample(t *testing.T) {
	t.Parallel()
	tw := newTestWriter(t)

	localProviderBinPath := ensureCompiledProvider(t)
	// skipLocalRunsWithoutCreds(t)
	// Module written to support the test.
	testProgram, err := filepath.Abs(filepath.Join("../", "examples", "aws-alb-example"))
	require.NoError(t, err)
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))
	integrationTest := newPulumiTest(t, testProgram, localPath)

	// Get a prefix for resource names
	prefix := generateTestResourcePrefix()

	// Set prefix via config
	integrationTest.SetConfig(t, "prefix", prefix)

	// Generate package
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/vpc/aws", "5.19.0", "vpcmod")
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/lambda/aws", "7.20.1", "lambdamod")
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/alb/aws", "9.14.0", "albmod")
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/s3-bucket/aws", "4.6.0", "bucketmod")

	upResult := integrationTest.Up(t,
		optup.Diff(),
		optup.ErrorProgressStreams(tw),
		optup.ProgressStreams(tw),
	)

	assert.Equal(t, &map[string]int{
		"create": 46,
	}, upResult.Summary.ResourceChanges)

	integrationTest.Preview(t,
		optpreview.Diff(),
		optpreview.ErrorProgressStreams(tw),
		optpreview.ProgressStreams(tw),
	)
}
