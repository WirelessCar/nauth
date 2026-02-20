/*
Copyright 2025.

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

package synadia

import (
	"testing"

	synadiav1alpha1 "github.com/WirelessCar/nauth/api/synadia/v1alpha1"
	nauthv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func toInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func toBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

func TestNatsLimitsFromAccount_nil_spec_returns_defaults(t *testing.T) {
	out := NatsLimitsFromAccount(nil)
	require.NotNil(t, out)
	assert.Equal(t, int64(DefaultSubs), toInt64(out.Subs))
	assert.Equal(t, int64(DefaultPayloadBytes), toInt64(out.Payload))
	assert.Equal(t, int64(DefaultDataUnlimited), toInt64(out.Data), "data rate unlimited by default")
	assert.Equal(t, int64(DefaultConn), toInt64(out.Conn))
	assert.Equal(t, int64(DefaultLeaf), toInt64(out.Leaf))
	assert.Equal(t, int64(DefaultImportUnlimited), toInt64(out.Import))
	assert.Equal(t, int64(DefaultExportUnlimited), toInt64(out.Export))
	assert.Equal(t, DefaultWildcard, toBool(out.Wildcard))
}

func TestNatsLimitsFromAccount_unlimited_minus_one_becomes_default(t *testing.T) {
	spec := &nauthv1alpha1.AccountSpec{
		NatsLimits: &nauthv1alpha1.NatsLimits{
			Subs:    ptr.To(int64(-1)),
			Data:    ptr.To(int64(-1)),
			Payload: ptr.To(int64(-1)),
		},
	}
	out := NatsLimitsFromAccount(spec)
	require.NotNil(t, out)
	assert.Equal(t, int64(DefaultSubs), toInt64(out.Subs))
	assert.Equal(t, int64(DefaultDataUnlimited), toInt64(out.Data), "data -1 (unlimited) becomes default")
	assert.Equal(t, int64(DefaultPayloadBytes), toInt64(out.Payload))
	assert.Equal(t, int64(DefaultConn), toInt64(out.Conn))
	assert.Equal(t, int64(DefaultLeaf), toInt64(out.Leaf))
	assert.Equal(t, int64(DefaultImportUnlimited), toInt64(out.Import))
	assert.Equal(t, int64(DefaultExportUnlimited), toInt64(out.Export))
	assert.Equal(t, DefaultWildcard, toBool(out.Wildcard))
}

func TestNatsLimitsFromAccount_explicit_values_preserved(t *testing.T) {
	spec := &nauthv1alpha1.AccountSpec{
		NatsLimits: &nauthv1alpha1.NatsLimits{
			Subs:    ptr.To(int64(10)),
			Data:    ptr.To(int64(2048)),
			Payload: ptr.To(int64(1024)),
		},
	}
	out := NatsLimitsFromAccount(spec)
	require.NotNil(t, out)
	assert.Equal(t, int64(10), toInt64(out.Subs))
	assert.Equal(t, int64(2048), toInt64(out.Data))
	assert.Equal(t, int64(1024), toInt64(out.Payload))
	assert.Equal(t, int64(DefaultConn), toInt64(out.Conn))
	assert.Equal(t, int64(DefaultLeaf), toInt64(out.Leaf))
	assert.Equal(t, int64(DefaultImportUnlimited), toInt64(out.Import))
	assert.Equal(t, int64(DefaultExportUnlimited), toInt64(out.Export))
	assert.Equal(t, DefaultWildcard, toBool(out.Wildcard))
}

func TestNatsLimitsFromAccount_conn_at_least_one(t *testing.T) {
	spec := &nauthv1alpha1.AccountSpec{
		NatsLimits: &nauthv1alpha1.NatsLimits{},
	}
	out := NatsLimitsFromAccount(spec)
	require.NotNil(t, out)
	assert.NotNil(t, out.Conn)
	assert.GreaterOrEqual(t, toInt64(out.Conn), int64(1), "conn must be at least 1 for API")
}

func TestNatsLimitsFromUser_nil_returns_defaults(t *testing.T) {
	out := NatsLimitsFromUser(nil)
	require.NotNil(t, out)
	assert.Equal(t, int64(DefaultSubs), toInt64(out.Subs))
	assert.Equal(t, int64(DefaultPayloadBytes), toInt64(out.Payload))
	assert.Equal(t, int64(DefaultDataUnlimited), toInt64(out.Data), "data rate unlimited by default")
	assert.Equal(t, int64(DefaultConn), toInt64(out.Conn))
	assert.Equal(t, int64(DefaultLeaf), toInt64(out.Leaf))
	assert.Equal(t, int64(DefaultImportUnlimited), toInt64(out.Import))
	assert.Equal(t, int64(DefaultExportUnlimited), toInt64(out.Export))
	assert.Equal(t, DefaultWildcard, toBool(out.Wildcard))
}

func TestTieredLimitsFromTieredLimit_nil_returns_nil(t *testing.T) {
	out := TieredLimitsFromTieredLimit(nil)
	assert.Nil(t, out)
}

func TestTieredLimitsFromTieredLimit_r1_r3_unlimited_minus_one_becomes_zero(t *testing.T) {
	tl := &synadiav1alpha1.TieredLimit{
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "acc", Namespace: "default"},
			R1: &synadiav1alpha1.TieredLimitTier{
				DiskStorage: ptr.To(int64(-1)),
				Streams:     ptr.To(int64(-1)),
				Consumer:    ptr.To(int64(-1)),
			},
			R3: &synadiav1alpha1.TieredLimitTier{
				DiskStorage: ptr.To(int64(1000)),
				Streams:     ptr.To(int64(5)),
				Consumer:    ptr.To(int64(-1)),
			},
		},
	}
	out := TieredLimitsFromTieredLimit(tl)
	require.NotNil(t, out)
	require.NotNil(t, out.R1)
	assert.Equal(t, int64(0), toInt64(out.R1.DiskStorage))
	assert.Equal(t, int64(0), toInt64(out.R1.Streams))
	assert.Equal(t, int64(0), toInt64(out.R1.Consumer))
	require.NotNil(t, out.R3)
	assert.Equal(t, int64(1000), toInt64(out.R3.DiskStorage))
	assert.Equal(t, int64(5), toInt64(out.R3.Streams))
	assert.Equal(t, int64(0), toInt64(out.R3.Consumer))
}

func TestTieredLimitsFromTieredLimit_explicit_values_preserved(t *testing.T) {
	tl := &synadiav1alpha1.TieredLimit{
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "acc"},
			R1: &synadiav1alpha1.TieredLimitTier{
				DiskStorage: ptr.To(int64(1 << 30)),
				Streams:     ptr.To(int64(10)),
				Consumer:    ptr.To(int64(5)),
			},
		},
	}
	out := TieredLimitsFromTieredLimit(tl)
	require.NotNil(t, out)
	require.NotNil(t, out.R1)
	assert.Equal(t, int64(1<<30), toInt64(out.R1.DiskStorage))
	assert.Equal(t, int64(10), toInt64(out.R1.Streams))
	assert.Equal(t, int64(5), toInt64(out.R1.Consumer))
}

func TestTieredLimitsFromTieredLimit_diskMaxStreamBytes_limited(t *testing.T) {
	tl := &synadiav1alpha1.TieredLimit{
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "acc"},
			R1: &synadiav1alpha1.TieredLimitTier{
				DiskMaxStreamBytes: ptr.To(int64(512)),
			},
		},
	}
	out := TieredLimitsFromTieredLimit(tl)
	require.NotNil(t, out)
	require.NotNil(t, out.R1)
	assert.Equal(t, int64(512), toInt64(out.R1.DiskMaxStreamBytes))
}

func TestTieredLimitsFromTieredLimit_diskMaxStreamBytes_minus_one_becomes_zero(t *testing.T) {
	tl := &synadiav1alpha1.TieredLimit{
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "acc"},
			R1: &synadiav1alpha1.TieredLimitTier{
				DiskMaxStreamBytes: ptr.To(int64(-1)),
			},
		},
	}
	out := TieredLimitsFromTieredLimit(tl)
	require.NotNil(t, out)
	require.NotNil(t, out.R1)
	assert.Equal(t, int64(0), toInt64(out.R1.DiskMaxStreamBytes))
}

func TestTieredLimitsFromTieredLimit_maxAckPending_unlimited(t *testing.T) {
	tl := &synadiav1alpha1.TieredLimit{
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "acc"},
			R1: &synadiav1alpha1.TieredLimitTier{
				MaxAckPending: ptr.To(int64(-1)),
			},
		},
	}
	out := TieredLimitsFromTieredLimit(tl)
	require.NotNil(t, out)
	require.NotNil(t, out.R1)
	assert.Equal(t, int64(-1), toInt64(out.R1.MaxAckPending))
}

func TestTieredLimitsFromTieredLimit_maxAckPending_nil_defaults_to_unlimited(t *testing.T) {
	tl := &synadiav1alpha1.TieredLimit{
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "acc"},
			R1:         &synadiav1alpha1.TieredLimitTier{},
		},
	}
	out := TieredLimitsFromTieredLimit(tl)
	require.NotNil(t, out)
	require.NotNil(t, out.R1)
	assert.Equal(t, int64(-1), toInt64(out.R1.MaxAckPending))
}

func TestTieredLimitsFromTieredLimit_maxAckPending_explicit_value(t *testing.T) {
	tl := &synadiav1alpha1.TieredLimit{
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "acc"},
			R1: &synadiav1alpha1.TieredLimitTier{
				MaxAckPending: ptr.To(int64(1000)),
			},
		},
	}
	out := TieredLimitsFromTieredLimit(tl)
	require.NotNil(t, out)
	require.NotNil(t, out.R1)
	assert.Equal(t, int64(1000), toInt64(out.R1.MaxAckPending))
}

func TestTieredLimitsFromTieredLimit_maxBytesRequired_default_true(t *testing.T) {
	tl := &synadiav1alpha1.TieredLimit{
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "acc"},
			R1:         &synadiav1alpha1.TieredLimitTier{},
		},
	}
	out := TieredLimitsFromTieredLimit(tl)
	require.NotNil(t, out)
	require.NotNil(t, out.R1)
	assert.True(t, toBool(out.R1.MaxBytesRequired), "default should be true")
}

func TestTieredLimitsFromTieredLimit_maxBytesRequired_explicit_false(t *testing.T) {
	tl := &synadiav1alpha1.TieredLimit{
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "acc"},
			R1: &synadiav1alpha1.TieredLimitTier{
				MaxBytesRequired: ptr.To(false),
			},
		},
	}
	out := TieredLimitsFromTieredLimit(tl)
	require.NotNil(t, out)
	require.NotNil(t, out.R1)
	assert.False(t, toBool(out.R1.MaxBytesRequired))
}
