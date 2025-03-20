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

// Package property exposes PropertyMap/PropertyValue helpers that did not make it into the official Pulumi Go SDK yet.
//
// See also: https://github.com/pulumi/pulumi/issues/18447
package property

import (
	"context"
	"errors"
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Attempts a lossless conversion of PropertyMap into a native Go SDK representation.
//
// The native Go SDK representation can then be used to call ctx.RegsiterResource and other methods.
//
// This is a workaround for https://github.com/pulumi/pulumi/issues/18447
//
// Known limitations:
//
// - first-class Output values with dependencies are not supported
// - resource references are not supported
// - PropertyMap{"foo": NewNullProperty()} may not turnaround but drop "foo"
func UnmarshalPropertyMap(ctx *pulumi.Context, v resource.PropertyMap) (pulumi.Map, error) {
	if v == nil {
		return nil, nil
	}

	var unmarshal func(resource.PropertyValue) (pulumi.Input, error)
	unmarshal = func(v resource.PropertyValue) (pulumi.Input, error) {
		switch {
		case v.IsNull():
			return nil, nil
		case v.IsBool():
			return pulumi.Bool(v.BoolValue()), nil
		case v.IsNumber():
			return pulumi.Float64(v.NumberValue()), nil
		case v.IsString():
			return pulumi.String(v.StringValue()), nil
		case v.IsArray():
			a := v.ArrayValue()
			r := make(pulumi.Array, len(a))
			for i, v := range a {
				uv, err := unmarshal(v)
				if err != nil {
					return nil, err
				}
				r[i] = uv
			}
			return r, nil
		case v.IsObject():
			m := v.ObjectValue()
			return UnmarshalPropertyMap(ctx, m)
		case v.IsAsset():
			asset := v.AssetValue()
			switch {
			case asset.IsPath():
				return pulumi.NewFileAsset(asset.Path), nil
			case asset.IsText():
				return pulumi.NewStringAsset(asset.Text), nil
			case asset.IsURI():
				return pulumi.NewRemoteAsset(asset.URI), nil
			}
			return nil, errors.New("expected asset to be one of File, String, or Remote; got none")
		case v.IsArchive():
			archive := v.ArchiveValue()
			secret := false
			switch {
			case archive.IsAssets():
				as := make(map[string]interface{})
				for k, v := range archive.Assets {
					a, asecret, err := unmarshalPropertyValue(ctx, resource.NewPropertyValue(v))
					secret = secret || asecret
					if err != nil {
						return nil, err
					}
					as[k] = a
				}
				return pulumi.NewAssetArchive(as), nil
			case archive.IsPath():
				return pulumi.NewFileArchive(archive.Path), nil
			case archive.IsURI():
				return pulumi.NewRemoteArchive(archive.URI), nil
			}
			return nil, errors.New("expected archive to be one of Assets, File, or Remote; got none")
		case v.IsResourceReference():
			contract.Failf("ResourceReference is not yet supported in UnmarshalPropertyValue")
			return nil, nil
		case v.IsComputed():
			return pulumi.UnsafeUnknownOutput(nil /*deps*/), nil
		case v.IsSecret():
			element, err := unmarshal(v.SecretValue().Element)
			if err != nil {
				return nil, err
			}
			return pulumi.ToSecret(element), nil
		case v.IsOutput():
			o := v.OutputValue()
			if !o.Known {
				return pulumi.UnsafeUnknownOutput(deps(o.Dependencies)), nil
			}

			element, err := unmarshal(o.Element)
			if err != nil {
				return nil, err
			}

			if o.Secret {
				return WithDeps(ctx.Context(), o.Dependencies, pulumi.ToSecret(element)), nil
			}

			return WithDeps(ctx.Context(), o.Dependencies, pulumi.ToOutput(element)), nil
		}

		return nil, fmt.Errorf("unknown property value %v", v)
	}

	m := make(pulumi.Map)
	for k, v := range v {
		uv, err := unmarshal(v)
		if err != nil {
			return nil, err
		}
		m[string(k)] = uv
	}
	return m, nil
}

// Helper function to attach dependency URNs to an existing output.
func WithDeps(ctx context.Context, urns []resource.URN, out pulumi.Input) pulumi.Input {
	if len(urns) == 0 {
		return out
	}
	return pulumi.OutputWithDependencies(ctx, pulumi.ToOutputWithContext(ctx, out), deps(urns)...)
}

// Broadcast deps to every element of a map.
func MapWithDeps(ctx context.Context, urns []resource.URN, out pulumi.Map) pulumi.Map {
	if len(urns) == 0 {
		return out
	}
	result := pulumi.Map{}
	for k, v := range out {
		result[k] = WithDeps(ctx, urns, v)
	}
	return result
}

func deps(urns []resource.URN) []pulumi.Resource {
	rr := []pulumi.Resource{}
	seen := map[resource.URN]struct{}{}

	for _, u := range urns {
		if _, ok := seen[u]; ok {
			continue
		}
		rr = append(rr, &depResource{urn: pulumi.URN(u)})
		seen[u] = struct{}{}
	}
	return rr
}

type depResource struct {
	pulumi.CustomResourceState
	urn pulumi.URN
}

func (d *depResource) URN() pulumi.URNOutput {
	return pulumi.URNPtr(d.urn).ToURNPtrOutput().Elem()
}

func unmarshalPropertyValue(ctx *pulumi.Context, v resource.PropertyValue) (interface{}, bool, error) {
	switch {
	case v.IsComputed():
		return nil, false, nil
	case v.IsOutput():
		if !v.OutputValue().Known {
			return nil, v.OutputValue().Secret, nil
		}
		ov, _, err := unmarshalPropertyValue(ctx, v.OutputValue().Element)
		if err != nil {
			return nil, false, err
		}
		return ov, v.OutputValue().Secret, nil
	case v.IsSecret():
		sv, _, err := unmarshalPropertyValue(ctx, v.SecretValue().Element)
		if err != nil {
			return nil, false, err
		}
		return sv, true, nil
	case v.IsArray():
		arr := v.ArrayValue()
		rv := make([]interface{}, len(arr))
		secret := false
		for i, e := range arr {
			ev, esecret, err := unmarshalPropertyValue(ctx, e)
			secret = secret || esecret
			if err != nil {
				return nil, false, err
			}
			rv[i] = ev
		}
		return rv, secret, nil
	case v.IsObject():
		m := make(map[string]interface{})
		secret := false
		for k, e := range v.ObjectValue() {
			ev, esecret, err := unmarshalPropertyValue(ctx, e)
			secret = secret || esecret
			if err != nil {
				return nil, false, err
			}
			m[string(k)] = ev
		}
		return m, secret, nil
	case v.IsAsset():
		asset := v.AssetValue()
		switch {
		case asset.IsPath():
			return pulumi.NewFileAsset(asset.Path), false, nil
		case asset.IsText():
			return pulumi.NewStringAsset(asset.Text), false, nil
		case asset.IsURI():
			return pulumi.NewRemoteAsset(asset.URI), false, nil
		}
		return nil, false, errors.New("expected asset to be one of File, String, or Remote; got none")
	case v.IsArchive():
		archive := v.ArchiveValue()
		secret := false
		switch {
		case archive.IsAssets():
			as := make(map[string]interface{})
			for k, v := range archive.Assets {
				a, asecret, err := unmarshalPropertyValue(ctx, resource.NewPropertyValue(v))
				secret = secret || asecret
				if err != nil {
					return nil, false, err
				}
				as[k] = a
			}
			return pulumi.NewAssetArchive(as), secret, nil
		case archive.IsPath():
			return pulumi.NewFileArchive(archive.Path), secret, nil
		case archive.IsURI():
			return pulumi.NewRemoteArchive(archive.URI), secret, nil
		}
		return nil, false, errors.New("expected asset to be one of File, String, or Remote; got none")
	case v.IsResourceReference():
		contract.Failf("ResourceReference is not yet supported in UnmarshalPropertyValue")
		return nil, false, nil
	default:
		return v.V, false, nil
	}
}

func MustUnmarshalPropertyMap(ctx *pulumi.Context, v resource.PropertyMap) pulumi.Map {
	m, err := UnmarshalPropertyMap(ctx, v)
	contract.AssertNoErrorf(err, "UnmarshalPropertyMap failed unexpectedly")
	return m
}
