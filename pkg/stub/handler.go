package stub

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/pkg/errors"
	"github.com/prometheus/common/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/previousnext/cloudfront-invalidation-operator/pkg/apis/cloudfront/v1alpha1"
)

const (
	// ConfigDistributionID used for looking up ConfigMap value.
	ConfigDistributionID = "cloudfront.distribution.id"
	// ConfigUserID used for looking up ConfigMap value.
	ConfigUserID = "cloudfront.iam.id"
	// ConfigUserSecret used for looking up ConfigMap value.
	ConfigUserSecret = "cloudfront.iam.secret"
)

// NewHandler to react to object events.
func NewHandler() sdk.Handler {
	return &Handler{}
}

// Handler of object events.
type Handler struct{}

// Handle object events.
func (h *Handler) Handle(ctx context.Context, event sdk.Event) error {
	switch o := event.Object.(type) {
	case *v1alpha1.Invalidation:
		err := invalidate(o)
		if err != nil {
			return errors.Wrap(err, "failed to process invalidation request")
		}
	}
	return nil
}

func invalidate(cr *v1alpha1.Invalidation) error {
	log.With("namespace", cr.ObjectMeta.Namespace).With("name", cr.ObjectMeta.Name).Infoln("Received invalidation request")

	config, err := rest.InClusterConfig()
	if err != nil {
		return errors.Wrap(err, "failed to get Kubernetes config")
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return errors.Wrap(err, "failed to get Kubernetes clientset")
	}

	log.With("namespace", cr.ObjectMeta.Namespace).With("name", cr.ObjectMeta.Name).Infoln("Loading ConfigMap")

	configmap, err := clientset.CoreV1().ConfigMaps(cr.ObjectMeta.Namespace).Get(cr.Spec.ConfigMap, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to load ConfigMap")
	}

	distribution, err := getConfig(ConfigDistributionID, configmap)
	if err != nil {
		return errors.Wrap(err, "distribution not found")
	}

	user, err := getConfig(ConfigUserID, configmap)
	if err != nil {
		return errors.Wrap(err, "user id not found")
	}

	secret, err := getConfig(ConfigUserSecret, configmap)
	if err != nil {
		return errors.Wrap(err, "user secret not found")
	}

	svc := cloudfront.New(session.New(&aws.Config{
		Credentials: credentials.NewStaticCredentials(user, secret, ""),
	}))

	log.With("namespace", cr.ObjectMeta.Namespace).With("name", cr.ObjectMeta.Name).Infoln("Submitting invalidation request")

	create, err := svc.CreateInvalidation(&cloudfront.CreateInvalidationInput{
		DistributionId: aws.String(distribution),
		InvalidationBatch: &cloudfront.InvalidationBatch{
			CallerReference: aws.String(time.Now().String()),
			Paths: &cloudfront.Paths{
				Quantity: aws.Int64(1),
				Items: []*string{
					aws.String(cr.Spec.Path),
				},
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to create invalidation")
	}

	log.With("namespace", cr.ObjectMeta.Namespace).With("name", cr.ObjectMeta.Name).Infoln("Waiting for invalidation to complete")

	// Wait for the invalidation to finish.
	limiter := time.Tick(time.Second / 10)

	for {
		<-limiter

		resp, err := svc.GetInvalidation(&cloudfront.GetInvalidationInput{
			DistributionId: aws.String(distribution),
			Id:             create.Invalidation.Id,
		})
		if err != nil {
			return errors.Wrap(err, "failed to create invalidation")
		}

		// See documentation for status codes.
		// https://docs.aws.amazon.com/cli/latest/reference/cloudfront/create-invalidation.html
		if *resp.Invalidation.Status == "Completed" {
			break
		}
	}

	log.With("namespace", cr.ObjectMeta.Namespace).With("name", cr.ObjectMeta.Name).Infoln("Invalidation finished")

	// Mark this invalidation as complete.
	cr.Status.Phase = v1alpha1.PhaseCompleted
	return sdk.Update(cr)
}

// Helper function to lookup a ConfigMap value by key.
func getConfig(want string, cfg *corev1.ConfigMap) (string, error) {
	for key, value := range cfg.Data {
		if key == want {
			return value, nil
		}
	}

	return "", errors.New("not found")
}
