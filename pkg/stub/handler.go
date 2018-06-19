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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/previousnext/cloudfront-invalidation-operator/pkg/apis/cloudfront/v1alpha1"
)

const (
	// ConfigDistributionID used for looking up ConfigMap value.
	ConfigDistributionID = "cloudfront.distribution.id"
	// ConfigCredentialID used for looking up ConfigMap value.
	ConfigCredentialID = "cloudfront.credential.id"
	// ConfigCredentialAccess used for looking up ConfigMap value.
	ConfigCredentialAccess = "cloudfront.credential.access"
	// StatusCompleted identifies an invalidation has been completed.
	StatusCompleted = "Completed"
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

	configMap, err := clientset.CoreV1().ConfigMaps(cr.ObjectMeta.Namespace).Get(cr.Spec.ConfigMap, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to load ConfigMap")
	}

	// Validate ConfigMap has all the values we require.
	if _, found := configMap.Data[ConfigDistributionID]; !found {
		return errors.New("distribution not found, skipping")
	}
	if _, found := configMap.Data[ConfigCredentialID]; !found {
		return errors.New("credential not found: id, skipping")
	}
	if _, found := configMap.Data[ConfigCredentialAccess]; !found {
		return errors.New("credential not found: access, skipping")
	}

	var (
		distribution     = configMap.Data[ConfigDistributionID]
		credentialID     = configMap.Data[ConfigCredentialID]
		credentialAccess = ConfigCredentialAccess
	)

	svc := cloudfront.New(session.New(&aws.Config{
		Credentials: credentials.NewStaticCredentials(credentialID, credentialAccess, ""),
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
		if *resp.Invalidation.Status == StatusCompleted {
			break
		}
	}

	log.With("namespace", cr.ObjectMeta.Namespace).With("name", cr.ObjectMeta.Name).Infoln("Invalidation finished")

	// Mark this invalidation as complete.
	cr.Status = v1alpha1.InvalidationStatus{
		ID:    *create.Invalidation.Id,
		Phase: StatusCompleted,
	}
	return sdk.Update(cr)
}
