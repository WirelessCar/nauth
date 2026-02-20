/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS FOR A PARTICULAR PURPOSE.
See the License for the specific language governing permissions and
limitations under the License.
*/

package synadia

import (
	synadiav1alpha1 "github.com/WirelessCar/nauth/api/synadia/v1alpha1"
	nauthv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
)

// Synadia default limits when CR uses -1 (unlimited). API may reject unlimited for some fields.
const (
	DefaultPayloadBytes    = 1 * 1024 * 1024 // 1 MiB
	DefaultSubs            = 1
	DefaultConn            = 1
	DefaultLeaf            = 0
	DefaultDataUnlimited   = -1 // Data (data rate) is unlimited by default
	DefaultImportUnlimited = -1
	DefaultExportUnlimited = -1
	DefaultWildcard        = true
)

// NatsLimitsFromAccount converts Account NatsLimits to API DTO. -1 (unlimited) is translated to Synadia defaults.
func NatsLimitsFromAccount(spec *nauthv1alpha1.AccountSpec) *NatsLimitsDTO {
	if spec == nil || spec.NatsLimits == nil {
		return defaultNatsLimits()
	}
	return natsLimitsFromCR(spec.NatsLimits.Subs, spec.NatsLimits.Data, spec.NatsLimits.Payload, nil, nil)
}

// NatsLimitsFromUser converts User NatsLimits to API DTO.
func NatsLimitsFromUser(spec *nauthv1alpha1.UserSpec) *NatsLimitsDTO {
	if spec == nil || spec.NatsLimits == nil {
		return defaultNatsLimits()
	}
	return natsLimitsFromCR(spec.NatsLimits.Subs, spec.NatsLimits.Data, spec.NatsLimits.Payload, nil, nil)
}

func defaultNatsLimits() *NatsLimitsDTO {
	return &NatsLimitsDTO{
		Subs:     int64Ptr(DefaultSubs),
		Payload:  int64Ptr(DefaultPayloadBytes),
		Data:     int64Ptr(DefaultDataUnlimited), // data rate unlimited by default
		Conn:     int64Ptr(DefaultConn),
		Leaf:     int64Ptr(DefaultLeaf),
		Import:   int64Ptr(DefaultImportUnlimited),
		Export:   int64Ptr(DefaultExportUnlimited),
		Wildcard: boolPtr(DefaultWildcard),
	}
}

func natsLimitsFromCR(subs, data, payload, conn, leaf *int64) *NatsLimitsDTO {
	out := &NatsLimitsDTO{}
	out.Subs = limitOrDefault(subs, DefaultSubs)
	out.Data = limitOrDefault(data, DefaultDataUnlimited)
	out.Payload = limitOrDefault(payload, DefaultPayloadBytes)
	if conn != nil {
		out.Conn = limitOrDefault(conn, DefaultConn)
	} else {
		out.Conn = int64Ptr(DefaultConn)
	}
	if out.Conn != nil && *out.Conn < 1 {
		*out.Conn = DefaultConn
	}
	if leaf != nil {
		out.Leaf = limitOrDefault(leaf, DefaultLeaf)
	} else {
		out.Leaf = int64Ptr(DefaultLeaf)
	}
	out.Import = int64Ptr(DefaultImportUnlimited)
	out.Export = int64Ptr(DefaultExportUnlimited)
	out.Wildcard = boolPtr(DefaultWildcard)
	return out
}

// limitOrDefault returns the value to send to the API: -1 (unlimited) becomes defaultVal; otherwise the CR value.
func limitOrDefault(v *int64, defaultVal int64) *int64 {
	if v == nil {
		return &defaultVal
	}
	if *v == -1 {
		return &defaultVal
	}
	return v
}

// TieredLimitsFromTieredLimit builds TieredLimitsDTO from the TieredLimit CR.
// -1 (unlimited) is translated to 0 for limited fields.
func TieredLimitsFromTieredLimit(tl *synadiav1alpha1.TieredLimit) *TieredLimitsDTO {
	if tl == nil {
		return nil
	}
	var r1, r3 *TieredTierDTO
	if tl.Spec.R1 != nil {
		r1 = tierDTOFromTier(tl.Spec.R1)
	}
	if tl.Spec.R3 != nil {
		r3 = tierDTOFromTier(tl.Spec.R3)
	}
	if r1 == nil && r3 == nil {
		return nil
	}
	return &TieredLimitsDTO{R1: r1, R3: r3}
}

func tierDTOFromTier(t *synadiav1alpha1.TieredLimitTier) *TieredTierDTO {
	out := &TieredTierDTO{}
	out.DiskStorage = tierLimitOrZero(t.DiskStorage)
	out.DiskMaxStreamBytes = tierLimitOrZero(t.DiskMaxStreamBytes)
	out.Streams = tierLimitOrZero(t.Streams)
	out.Consumer = tierLimitOrZero(t.Consumer)
	out.MaxAckPending = tierMaxAckPendingOrUnlimited(t.MaxAckPending)
	out.MaxBytesRequired = tierMaxBytesRequired(t.MaxBytesRequired)
	return out
}

// tierLimitOrZero: -1 (unlimited) → 0 for Synadia limited fields; otherwise use value.
func tierLimitOrZero(v *int64) *int64 {
	if v == nil {
		return int64Ptr(0)
	}
	if *v == -1 {
		return int64Ptr(0)
	}
	return v
}

// tierMaxAckPendingOrUnlimited: nil or -1 → -1 (unlimited); otherwise use value.
func tierMaxAckPendingOrUnlimited(v *int64) *int64 {
	if v == nil || *v == -1 {
		return int64Ptr(-1)
	}
	return v
}

// tierMaxBytesRequired: nil → true (default); otherwise use value.
func tierMaxBytesRequired(v *bool) *bool {
	if v == nil {
		return boolPtr(true)
	}
	return v
}

func int64Ptr(n int64) *int64 {
	return &n
}

func boolPtr(b bool) *bool {
	return &b
}
