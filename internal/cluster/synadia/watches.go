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
	"context"

	synadiav1alpha1 "github.com/WirelessCar/nauth/api/synadia/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TieredLimitToAccount maps a TieredLimit event to the Account it references.
// Use with handler.EnqueueRequestsFromMapFunc to trigger Account reconciliation
// when a TieredLimit changes.
func TieredLimitToAccount(_ context.Context, obj client.Object) []ctrl.Request {
	tl, ok := obj.(*synadiav1alpha1.TieredLimit)
	if !ok {
		return nil
	}
	ns := tl.Spec.AccountRef.Namespace
	if ns == "" {
		ns = tl.GetNamespace()
	}
	if tl.Spec.AccountRef.Name == "" {
		return nil
	}
	return []ctrl.Request{
		{NamespacedName: client.ObjectKey{Namespace: ns, Name: tl.Spec.AccountRef.Name}},
	}
}
