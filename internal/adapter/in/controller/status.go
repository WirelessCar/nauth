package controller

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/WirelessCar-WDP/nauth/internal/core/domain/errs"
	"github.com/WirelessCar-WDP/nauth/internal/core/domain/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type Object interface {
	metav1.Object
	runtime.Object
	GetConditions() *[]metav1.Condition
}

type StatusReporter struct {
	client.Client
	Recorder record.EventRecorder
}

func (s *StatusReporter) Result(ctx context.Context, object Object, err error) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if err == nil {
		meta.SetStatusCondition(object.GetConditions(), metav1.Condition{
			Type:    types.ControllerTypeReady,
			Status:  metav1.ConditionTrue,
			Reason:  types.ControllerReasonReconciled,
			Message: "Successfully reconciled",
		})

		if updateErr := s.Status().Update(ctx, object); updateErr != nil {
			log.Info("Failed to update reconciled condition", "name", object.GetGenerateName(), "updateError", updateErr)
			return ctrl.Result{}, updateErr
		}

		// Spreading out the requeue to avoid all being queued at the same time
		return ctrl.Result{
			RequeueAfter: time.Duration(float64(5*time.Minute) * (0.9 + 0.2*rand.Float64())),
		}, nil
	}

	if errors.Is(err, errs.ErrUpdateFailed) {
		return ctrl.Result{}, err
	}

	s.Recorder.Eventf(object, v1.EventTypeWarning, types.ControllerReasonErrored, err.Error())

	meta.SetStatusCondition(object.GetConditions(), metav1.Condition{
		Type:    types.ControllerTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  types.ControllerReasonErrored,
		Message: err.Error(),
	})

	if updateErr := s.Status().Update(ctx, object); updateErr != nil {
		log.Info("Failed to update error condition", "name", object.GetGenerateName(), "updateError", updateErr, "originalError", err)
		return ctrl.Result{}, updateErr
	}

	return ctrl.Result{}, err
}
