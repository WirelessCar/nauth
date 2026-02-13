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

package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	synadiav1alpha1 "github.com/WirelessCar/nauth/api/synadia/v1alpha1"
	"github.com/WirelessCar/nauth/internal/cluster/synadia"
	"github.com/WirelessCar/nauth/internal/k8s/configmap"
	"github.com/WirelessCar/nauth/internal/k8s/secret"
)

var _ = Describe("System Controller", func() {
	const (
		systemNamespace = "system-test-ns"
		systemName      = "ngs"
	)

	var (
		ctx                  context.Context
		apiServer            *httptest.Server
		secretClient         *secret.Client
		configmapClient      *configmap.Client
		systemReconciler     *SystemReconciler
		systemNamespacedName k8stypes.NamespacedName
	)

	BeforeEach(func() {
		ctx = context.Background()
		systemNamespacedName = k8stypes.NamespacedName{Name: systemName, Namespace: systemNamespace}

		By("creating secret and configmap clients with test namespace")
		secretClient = secret.NewClient(k8sClient, secret.WithControllerNamespace("default"))
		configmapClient = configmap.NewClient(k8sClient)

		By("ensuring namespace exists")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: systemNamespace}}
		_ = k8sClient.Create(ctx, ns)
	})

	When("reconciling a System with spec.teamId and API returns systems", func() {
		BeforeEach(func() {
			apiServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/teams/team-123/systems") && r.Method == http.MethodGet {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(synadia.ListSystemsResponse{
						Systems: []synadia.SystemItem{
							{ID: "sys-ngs-id", Name: "NGS"},
							{ID: "sys-other", Name: "Other"},
						},
					})
					return
				}
				http.NotFound(w, r)
			}))

			By("creating API credentials secret")
			tokenSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "api-creds", Namespace: systemNamespace},
				Data:       map[string][]byte{"token": []byte("bearer-token")},
				StringData: map[string]string{"token": "bearer-token"},
			}
			Expect(k8sClient.Create(ctx, tokenSecret)).To(Succeed())

			By("creating System CR with teamId and systemSelector")
			sys := &synadiav1alpha1.System{
				ObjectMeta: metav1.ObjectMeta{Name: systemName, Namespace: systemNamespace},
				Spec: synadiav1alpha1.SystemSpec{
					TeamID:         "team-123",
					SystemSelector: synadiav1alpha1.SystemSelector{Name: "NGS"},
					APICredentialsSecretRef: synadiav1alpha1.SecretKeyReference{
						Name: "api-creds", Namespace: systemNamespace, Key: "token",
					},
					APIEndpoint: strings.TrimSuffix(apiServer.URL, "/"),
				},
			}
			Expect(k8sClient.Create(ctx, sys)).To(Succeed())

			fakeRecorder := events.NewFakeRecorder(5)
			systemReconciler = NewSystemReconciler(
				k8sClient,
				k8sClient.Scheme(),
				secretClient,
				configmapClient,
				fakeRecorder,
			)
		})

		AfterEach(func() {
			if apiServer != nil {
				apiServer.Close()
			}
		})

		It("resolves systemId and writes it to status", func() {
			_, err := systemReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: systemNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			sys := &synadiav1alpha1.System{}
			Expect(k8sClient.Get(ctx, systemNamespacedName, sys)).To(Succeed())
			Expect(sys.Status.SystemID).To(Equal("sys-ngs-id"))
			Expect(sys.Status.Conditions).NotTo(BeEmpty())
			ready := findCondition(sys.Status.Conditions, "Ready")
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	When("resolving teamId from teamIdFrom ConfigMap", func() {
		const systemNameCM = "ngs-cm" // unique name so it does not collide with first context's System

		BeforeEach(func() {
			apiServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/teams/team-from-cm/systems") {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(synadia.ListSystemsResponse{
						Systems: []synadia.SystemItem{{ID: "sys-1", Name: "NGS"}},
					})
					return
				}
				http.NotFound(w, r)
			}))

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "team-id-cm", Namespace: systemNamespace},
				Data:       map[string]string{"teamId": "team-from-cm"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			tokenSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "api-creds-cm", Namespace: systemNamespace},
				StringData: map[string]string{"token": "t"},
			}
			Expect(k8sClient.Create(ctx, tokenSecret)).To(Succeed())

			sys := &synadiav1alpha1.System{
				ObjectMeta: metav1.ObjectMeta{Name: systemNameCM, Namespace: systemNamespace},
				Spec: synadiav1alpha1.SystemSpec{
					TeamIDFrom: &synadiav1alpha1.TeamIDFromReference{
						Kind: "ConfigMap", Name: "team-id-cm", Key: "teamId",
					},
					SystemSelector: synadiav1alpha1.SystemSelector{Name: "NGS"},
					APICredentialsSecretRef: synadiav1alpha1.SecretKeyReference{
						Name: "api-creds-cm", Key: "token",
					},
					APIEndpoint: apiServer.URL,
				},
			}
			Expect(k8sClient.Create(ctx, sys)).To(Succeed())

			systemReconciler = NewSystemReconciler(
				k8sClient, k8sClient.Scheme(), secretClient, configmapClient, events.NewFakeRecorder(5),
			)
		})

		AfterEach(func() {
			if apiServer != nil {
				apiServer.Close()
			}
		})

		It("resolves teamId from ConfigMap and writes systemId to status", func() {
			nn := k8stypes.NamespacedName{Name: systemNameCM, Namespace: systemNamespace}
			_, err := systemReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			sys := &synadiav1alpha1.System{}
			Expect(k8sClient.Get(ctx, nn, sys)).To(Succeed())
			Expect(sys.Status.SystemID).To(Equal("sys-1"))
		})
	})
})

func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
