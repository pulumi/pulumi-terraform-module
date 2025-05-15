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
	"os"
	"path/filepath"
	"testing"

	"github.com/hexops/autogold/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/providertest/pulumitest/opttest"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
)

func Test_RdsExample(t *testing.T) {
	t.Parallel()

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
		optup.ErrorProgressStreams(os.Stderr),
		optup.ProgressStreams(os.Stdout),
	)

	integrationTest.Preview(t, optpreview.Diff(), optpreview.ExpectNoChanges(),
		optpreview.ErrorProgressStreams(os.Stderr),
		optpreview.ProgressStreams(os.Stdout),
	)

	integrationTest.Destroy(t)
}

func Test_EksExample(t *testing.T) {
	t.Parallel()

	localProviderBinPath := ensureCompiledProvider(t)
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
		optup.ErrorProgressStreams(os.Stderr),
		optup.ProgressStreams(os.Stdout),
	)

	integrationTest.Preview(t, optpreview.Diff(), optpreview.ExpectNoChanges(),
		optpreview.ErrorProgressStreams(os.Stderr),
		optpreview.ProgressStreams(os.Stdout),
	)
}

func Test_AlbExample(t *testing.T) {
	t.Parallel()
	tw := newTestWriter(t)

	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)
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
		// 4 ModuleState resources account for 46+4=50.
		"create": conditionalCount(50, 46),
	}, upResult.Summary.ResourceChanges)

	if !viewsEnabled {
		// Due to some issues specific to TF, the first preview is non-empty but detects some drift on the
		// lambda resource. If this gets fixed in the future the test may evolve as needed.
		resourceDiffs := runPreviewWithPlanDiff(t, integrationTest, "module.test-lambda.null_resource.archive[0]")

		// Ignore source_code_hash as it is unstable across dev machines and CI.
		fn := "module.test-lambda.aws_lambda_function.this[0]"
		resourceDiffs[fn].(map[string]any)["diff"].(apitype.PlanDiffV1).Updates["source_code_hash"] = "*"

		autogold.Expect(map[string]interface{}{"module.test-lambda.aws_lambda_function.this[0]": map[string]interface{}{
			"diff": apitype.PlanDiffV1{
				Adds: map[string]interface{}{"layers": []interface{}{}},
				Updates: map[string]interface{}{
					"last_modified":        "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
					"qualified_arn":        "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
					"qualified_invoke_arn": "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
					"source_code_hash":     "*",
					"version":              "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
				},
			},
			"steps": []apitype.OpType{apitype.OpType("update")},
		}}).Equal(t, resourceDiffs)
	} else {
		// There are some issues currently with views and update plans, making detailed asserts unreliable.
		// Instead we run preview directly and check the result.
		//
		// TODO[pulumi/pulumi-terraform-module#332]: views do not currently detect drift, so the result is an
		// empty preview here. This may change to an Update plan once the issue with drift detection is fixed.
		previewResult := integrationTest.Preview(t,
			optpreview.Diff(),
			optpreview.ErrorProgressStreams(tw),
			optpreview.ProgressStreams(tw),
		)
		autogold.Expect(map[apitype.OpType]int{
			apitype.OpType("same"): 46},
		).Equal(t, previewResult.ChangeSummary)
	}
}
