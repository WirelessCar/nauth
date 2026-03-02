package main

import (
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/stretchr/testify/require"
)

func Test_parseNatsClusterRef_ShouldSucceed(t *testing.T) {

	testCases := []struct {
		name   string
		value  string
		expect *v1alpha1.NatsClusterRef
	}{
		{
			name:  "namespace and name",
			value: "my-namespace/my-cluster",
			expect: &v1alpha1.NatsClusterRef{
				Name:      "my-cluster",
				Namespace: "my-namespace",
			},
		},
		{
			name:  "namespace and name with only numbers",
			value: "0/1",
			expect: &v1alpha1.NatsClusterRef{
				Name:      "1",
				Namespace: "0",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := parseNatsClusterRef(testCase.value)

			require.NoError(t, err)
			require.Equal(t, testCase.expect, result)
		})
	}
}

func Test_parseNatsClusterRef_ShouldFail(t *testing.T) {

	testCases := []struct {
		name  string
		value string
	}{
		{
			name:  "empty string/undefined",
			value: "",
		},
		{
			name:  "name only",
			value: "my-cluster",
		},
		{
			name:  "separator without namespace",
			value: "/my-cluster",
		},
		{
			name:  "only namespace",
			value: "my-namespace/",
		},
		{
			name:  "invalid namespace char",
			value: "my.namespace/my-cluster",
		},
		{
			name:  "invalid name char",
			value: "my-namespace/my_cluster",
		},
		{
			name:  "too many segments",
			value: "ns1/ns2/cluster",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := parseNatsClusterRef(testCase.value)

			require.Error(t, err)
			require.Nil(t, result)
		})
	}
}
