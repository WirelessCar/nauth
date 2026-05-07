package controller

import (
	"testing"
	"time"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_toNAuthExportGroups_ShouldSortExportsByCreationTimeThenName(t *testing.T) {
	older := metav1.NewTime(time.Date(2026, time.May, 7, 10, 0, 0, 0, time.UTC))
	newer := metav1.NewTime(time.Date(2026, time.May, 7, 11, 0, 0, 0, time.UTC))
	exports := &v1alpha1.AccountExportList{
		Items: []v1alpha1.AccountExport{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "my-account-export-b",
					Namespace:         "default",
					UID:               "uid-b",
					CreationTimestamp: older,
				},
				Status: v1alpha1.AccountExportStatus{
					DesiredClaim: &v1alpha1.AccountExportClaim{
						ObservedGeneration: 1,
						Rules: []v1alpha1.AccountExportRule{
							{
								Name:    "conflicting-b",
								Subject: "conflict.*",
								Type:    v1alpha1.Stream,
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "my-account-export-a",
					Namespace:         "default",
					UID:               "uid-a",
					CreationTimestamp: newer,
				},
				Status: v1alpha1.AccountExportStatus{
					DesiredClaim: &v1alpha1.AccountExportClaim{
						ObservedGeneration: 1,
						Rules: []v1alpha1.AccountExportRule{
							{
								Name:    "conflicting-a",
								Subject: "conflict.>",
								Type:    v1alpha1.Stream,
							},
						},
					},
				},
			},
		},
	}

	groups, refs, err := toNAuthExportGroups(exports)

	require.NoError(t, err)
	require.Len(t, groups, 2)
	require.Len(t, refs, 2)
	require.Equal(t, "my-account-export-b", groups[0].Name)
	require.Equal(t, "my-account-export-a", groups[1].Name)
	require.Equal(t, "uid-b", string(refs[0].UID))
	require.Equal(t, "uid-a", string(refs[1].UID))
}

func Test_toNAuthExportGroups_ShouldUseNameAsTieBreakerWhenCreationTimesMatch(t *testing.T) {
	created := metav1.NewTime(time.Date(2026, time.May, 7, 10, 0, 0, 0, time.UTC))
	exports := &v1alpha1.AccountExportList{
		Items: []v1alpha1.AccountExport{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "my-account-export-b",
					Namespace:         "default",
					UID:               "uid-b",
					CreationTimestamp: created,
				},
				Status: v1alpha1.AccountExportStatus{
					DesiredClaim: &v1alpha1.AccountExportClaim{
						ObservedGeneration: 1,
						Rules: []v1alpha1.AccountExportRule{
							{
								Name:    "conflicting-b",
								Subject: "conflict.*",
								Type:    v1alpha1.Stream,
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "my-account-export-a",
					Namespace:         "default",
					UID:               "uid-a",
					CreationTimestamp: created,
				},
				Status: v1alpha1.AccountExportStatus{
					DesiredClaim: &v1alpha1.AccountExportClaim{
						ObservedGeneration: 1,
						Rules: []v1alpha1.AccountExportRule{
							{
								Name:    "conflicting-a",
								Subject: "conflict.>",
								Type:    v1alpha1.Stream,
							},
						},
					},
				},
			},
		},
	}

	groups, refs, err := toNAuthExportGroups(exports)

	require.NoError(t, err)
	require.Equal(t, "my-account-export-a", groups[0].Name)
	require.Equal(t, "my-account-export-b", groups[1].Name)
	require.Equal(t, "uid-a", string(refs[0].UID))
	require.Equal(t, "uid-b", string(refs[1].UID))
}

func Test_toNAuthImportGroups_ShouldSortImportsByCreationTimeThenName(t *testing.T) {
	older := metav1.NewTime(time.Date(2026, time.May, 7, 10, 0, 0, 0, time.UTC))
	newer := metav1.NewTime(time.Date(2026, time.May, 7, 11, 0, 0, 0, time.UTC))
	imports := &v1alpha1.AccountImportList{
		Items: []v1alpha1.AccountImport{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "my-account-import-b",
					Namespace:         "default",
					UID:               "uid-b",
					CreationTimestamp: older,
				},
				Status: v1alpha1.AccountImportStatus{
					DesiredClaim: &v1alpha1.AccountImportClaim{
						ObservedGeneration: 1,
						Rules: []v1alpha1.AccountImportRuleDerived{
							{
								AccountImportRule: v1alpha1.AccountImportRule{
									Name:    "conflicting-b",
									Subject: "conflict.*",
									Type:    v1alpha1.Stream,
								},
								Account: "ACCE",
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "my-account-import-a",
					Namespace:         "default",
					UID:               "uid-a",
					CreationTimestamp: newer,
				},
				Status: v1alpha1.AccountImportStatus{
					DesiredClaim: &v1alpha1.AccountImportClaim{
						ObservedGeneration: 1,
						Rules: []v1alpha1.AccountImportRuleDerived{
							{
								AccountImportRule: v1alpha1.AccountImportRule{
									Name:    "conflicting-a",
									Subject: "conflict.>",
									Type:    v1alpha1.Stream,
								},
								Account: "ACCE",
							},
						},
					},
				},
			},
		},
	}

	groups, refs, err := toNAuthImportGroups(imports)

	require.NoError(t, err)
	require.Len(t, groups, 2)
	require.Len(t, refs, 2)
	require.Equal(t, "my-account-import-b", groups[0].Name)
	require.Equal(t, "my-account-import-a", groups[1].Name)
	require.Equal(t, "uid-b", string(refs[0].UID))
	require.Equal(t, "uid-a", string(refs[1].UID))
}

func Test_toNAuthImportGroups_ShouldUseNameAsTieBreakerWhenCreationTimesMatch(t *testing.T) {
	created := metav1.NewTime(time.Date(2026, time.May, 7, 10, 0, 0, 0, time.UTC))
	imports := &v1alpha1.AccountImportList{
		Items: []v1alpha1.AccountImport{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "my-account-import-b",
					Namespace:         "default",
					UID:               "uid-b",
					CreationTimestamp: created,
				},
				Status: v1alpha1.AccountImportStatus{
					DesiredClaim: &v1alpha1.AccountImportClaim{
						ObservedGeneration: 1,
						Rules: []v1alpha1.AccountImportRuleDerived{
							{
								AccountImportRule: v1alpha1.AccountImportRule{
									Name:    "conflicting-b",
									Subject: "conflict.*",
									Type:    v1alpha1.Stream,
								},
								Account: "ACCE",
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "my-account-import-a",
					Namespace:         "default",
					UID:               "uid-a",
					CreationTimestamp: created,
				},
				Status: v1alpha1.AccountImportStatus{
					DesiredClaim: &v1alpha1.AccountImportClaim{
						ObservedGeneration: 1,
						Rules: []v1alpha1.AccountImportRuleDerived{
							{
								AccountImportRule: v1alpha1.AccountImportRule{
									Name:    "conflicting-a",
									Subject: "conflict.>",
									Type:    v1alpha1.Stream,
								},
								Account: "ACCE",
							},
						},
					},
				},
			},
		},
	}

	groups, refs, err := toNAuthImportGroups(imports)

	require.NoError(t, err)
	require.Equal(t, "my-account-import-a", groups[0].Name)
	require.Equal(t, "my-account-import-b", groups[1].Name)
	require.Equal(t, "uid-a", string(refs[0].UID))
	require.Equal(t, "uid-b", string(refs[1].UID))
}
