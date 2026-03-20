package controller

import (
	"context"
	"math/rand/v2"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type Object interface {
	metav1.Object
	runtime.Object
	GetConditions() *[]metav1.Condition
}

type statusReporter struct {
	client   client.StatusClient
	Recorder events.EventRecorder
}

func newStatusReporter(k8sClient client.StatusClient, recorder events.EventRecorder) *statusReporter {
	return &statusReporter{
		client:   k8sClient,
		Recorder: recorder,
	}
}

func (s *statusReporter) status(ctx context.Context, object Object) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	meta.SetStatusCondition(object.GetConditions(), metav1.Condition{
		Type:    controllerTypeReady,
		Status:  metav1.ConditionTrue,
		Reason:  controllerReasonReconciled,
		Message: "Successfully reconciled",
	})

	if err := s.client.Status().Update(ctx, object); err != nil {
		log.Info("Failed to update reconciled condition", "name", object.GetGenerateName(), "updateError", err)
		return ctrl.Result{}, err
	}

	// Spreading out the requeue to avoid all being queued at the same time
	return ctrl.Result{
		RequeueAfter: time.Duration(float64(5*time.Minute) * (0.9 + 0.2*rand.Float64())),
	}, nil
}

func (s *statusReporter) error(ctx context.Context, regarding Object, err error) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	s.Recorder.Eventf(regarding, nil, v1.EventTypeWarning, controllerReasonErrored, controllerActionReconciled, err.Error())

	meta.SetStatusCondition(regarding.GetConditions(), metav1.Condition{
		Type:    controllerTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  controllerReasonErrored,
		Message: err.Error(),
	})

	if updateErr := s.client.Status().Update(ctx, regarding); updateErr != nil {
		log.Info("Failed to update error condition", "name", regarding.GetGenerateName(), "updateError", updateErr, "originalError", err)
		return ctrl.Result{}, updateErr
	}

	return ctrl.Result{}, err
}
