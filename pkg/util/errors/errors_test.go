package errors_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"

	. "github.com/onsi/gomega"
)

// testNetError implements net.Error for testing network error classification.
type testNetError struct {
	timeout bool
}

func (e *testNetError) Error() string   { return "network error" }
func (e *testNetError) Timeout() bool   { return e.timeout }
func (e *testNetError) Temporary() bool { return false }

func TestClassify(t *testing.T) {
	t.Run("should classify unauthorized as authentication", func(t *testing.T) {
		g := NewWithT(t)
		err := apierrors.NewUnauthorized("token expired")
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryAuthentication)))
		g.Expect(result).To(HaveField("Code", Equal("AUTH_FAILED")))
		g.Expect(result).To(HaveField("Retriable", BeFalse()))
		g.Expect(result).To(HaveField("Suggestion", Not(BeEmpty())))
		g.Expect(result.Unwrap()).To(Equal(err))
	})

	t.Run("should classify forbidden as authorization", func(t *testing.T) {
		g := NewWithT(t)
		gr := schema.GroupResource{Group: "apps", Resource: "deployments"}
		err := apierrors.NewForbidden(gr, "test", errors.New("access denied"))
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryAuthorization)))
		g.Expect(result).To(HaveField("Code", Equal("AUTHZ_DENIED")))
		g.Expect(result).To(HaveField("Retriable", BeFalse()))
	})

	t.Run("should classify not found", func(t *testing.T) {
		g := NewWithT(t)
		gr := schema.GroupResource{Group: "apps", Resource: "deployments"}
		err := apierrors.NewNotFound(gr, "my-deploy")
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryNotFound)))
		g.Expect(result).To(HaveField("Code", Equal("NOT_FOUND")))
		g.Expect(result).To(HaveField("Retriable", BeFalse()))
	})

	t.Run("should classify conflict as retriable", func(t *testing.T) {
		g := NewWithT(t)
		gr := schema.GroupResource{Group: "apps", Resource: "deployments"}
		err := apierrors.NewConflict(gr, "my-deploy", errors.New("version changed"))
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryConflict)))
		g.Expect(result).To(HaveField("Code", Equal("CONFLICT")))
		g.Expect(result).To(HaveField("Retriable", BeTrue()))
	})

	t.Run("should classify server timeout as timeout", func(t *testing.T) {
		g := NewWithT(t)
		gr := schema.GroupResource{Group: "apps", Resource: "deployments"}
		err := apierrors.NewServerTimeout(gr, "list", 5)
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryTimeout)))
		g.Expect(result).To(HaveField("Code", Equal("SERVER_TIMEOUT")))
		g.Expect(result).To(HaveField("Retriable", BeTrue()))
	})

	t.Run("should classify service unavailable as server", func(t *testing.T) {
		g := NewWithT(t)
		err := apierrors.NewServiceUnavailable("overloaded")
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryServer)))
		g.Expect(result).To(HaveField("Code", Equal("SERVER_UNAVAILABLE")))
		g.Expect(result).To(HaveField("Retriable", BeTrue()))
	})

	t.Run("should classify internal server error as server", func(t *testing.T) {
		g := NewWithT(t)
		err := apierrors.NewInternalError(errors.New("server crashed"))
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryServer)))
		g.Expect(result).To(HaveField("Code", Equal("SERVER_ERROR")))
		g.Expect(result).To(HaveField("Retriable", BeTrue()))
	})

	t.Run("should classify already exists as conflict", func(t *testing.T) {
		g := NewWithT(t)
		gr := schema.GroupResource{Group: "apps", Resource: "deployments"}
		err := apierrors.NewAlreadyExists(gr, "my-deploy")
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryConflict)))
		g.Expect(result).To(HaveField("Code", Equal("ALREADY_EXISTS")))
		g.Expect(result).To(HaveField("Retriable", BeFalse()))
	})

	t.Run("should classify invalid as validation", func(t *testing.T) {
		g := NewWithT(t)
		gk := schema.GroupKind{Group: "apps", Kind: "Deployment"}
		err := apierrors.NewInvalid(gk, "my-deploy", field.ErrorList{})
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryValidation)))
		g.Expect(result).To(HaveField("Code", Equal("INVALID")))
		g.Expect(result).To(HaveField("Retriable", BeFalse()))
	})

	t.Run("should classify bad request as validation", func(t *testing.T) {
		g := NewWithT(t)
		err := apierrors.NewBadRequest("malformed input")
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryValidation)))
		g.Expect(result).To(HaveField("Code", Equal("BAD_REQUEST")))
		g.Expect(result).To(HaveField("Retriable", BeFalse()))
	})

	t.Run("should classify method not supported as validation", func(t *testing.T) {
		g := NewWithT(t)
		gr := schema.GroupResource{Group: "apps", Resource: "deployments"}
		err := apierrors.NewMethodNotSupported(gr, "PATCH")
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryValidation)))
		g.Expect(result).To(HaveField("Code", Equal("METHOD_NOT_SUPPORTED")))
		g.Expect(result).To(HaveField("Retriable", BeFalse()))
	})

	t.Run("should classify not acceptable as validation", func(t *testing.T) {
		g := NewWithT(t)
		gr := schema.GroupResource{Group: "apps", Resource: "deployments"}
		err := apierrors.NewGenericServerResponse(http.StatusNotAcceptable, "GET", gr, "", "not acceptable", 0, false)
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryValidation)))
		g.Expect(result).To(HaveField("Code", Equal("NOT_ACCEPTABLE")))
		g.Expect(result).To(HaveField("Retriable", BeFalse()))
	})

	t.Run("should classify unsupported media type as validation", func(t *testing.T) {
		g := NewWithT(t)
		gr := schema.GroupResource{Group: "apps", Resource: "deployments"}
		err := apierrors.NewGenericServerResponse(http.StatusUnsupportedMediaType, "POST", gr, "", "unsupported", 0, false)
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryValidation)))
		g.Expect(result).To(HaveField("Code", Equal("UNSUPPORTED_MEDIA_TYPE")))
		g.Expect(result).To(HaveField("Retriable", BeFalse()))
	})

	t.Run("should classify request entity too large as validation", func(t *testing.T) {
		g := NewWithT(t)
		err := apierrors.NewRequestEntityTooLargeError("payload too big")
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryValidation)))
		g.Expect(result).To(HaveField("Code", Equal("REQUEST_TOO_LARGE")))
		g.Expect(result).To(HaveField("Retriable", BeFalse()))
	})

	t.Run("should classify gone as server", func(t *testing.T) {
		g := NewWithT(t)
		gr := schema.GroupResource{Group: "apps", Resource: "deployments"}
		err := apierrors.NewGenericServerResponse(http.StatusGone, "GET", gr, "", "resource expired", 0, false)
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryServer)))
		g.Expect(result).To(HaveField("Code", Equal("GONE")))
		g.Expect(result).To(HaveField("Retriable", BeTrue()))
	})

	t.Run("should classify resource expired as server", func(t *testing.T) {
		g := NewWithT(t)
		err := apierrors.NewResourceExpired("watch expired")
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryServer)))
		g.Expect(result).To(HaveField("Code", Equal("RESOURCE_EXPIRED")))
		g.Expect(result).To(HaveField("Retriable", BeTrue()))
	})

	t.Run("should classify timeout error as timeout", func(t *testing.T) {
		g := NewWithT(t)
		err := apierrors.NewTimeoutError("gateway timeout", 5)
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryTimeout)))
		g.Expect(result).To(HaveField("Code", Equal("GATEWAY_TIMEOUT")))
		g.Expect(result).To(HaveField("Retriable", BeTrue()))
	})

	t.Run("should classify too many requests as server", func(t *testing.T) {
		g := NewWithT(t)
		err := apierrors.NewTooManyRequests("rate limited", 30)
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryServer)))
		g.Expect(result).To(HaveField("Code", Equal("RATE_LIMITED")))
		g.Expect(result).To(HaveField("Retriable", BeTrue()))
	})

	t.Run("should classify unexpected server error as server", func(t *testing.T) {
		g := NewWithT(t)
		gr := schema.GroupResource{Group: "apps", Resource: "deployments"}
		err := apierrors.NewGenericServerResponse(http.StatusInternalServerError, "GET", gr, "", "unexpected", 0, true)
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryServer)))
		g.Expect(result).To(HaveField("Code", Equal("UNEXPECTED_SERVER_ERROR")))
		g.Expect(result).To(HaveField("Retriable", BeTrue()))
	})

	t.Run("should classify unexpected object error as server", func(t *testing.T) {
		g := NewWithT(t)
		err := &apierrors.UnexpectedObjectError{Object: &metav1.Status{Status: metav1.StatusFailure}}
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryServer)))
		g.Expect(result).To(HaveField("Code", Equal("UNEXPECTED_OBJECT")))
		g.Expect(result).To(HaveField("Retriable", BeFalse()))
	})

	t.Run("should classify store read error as server", func(t *testing.T) {
		g := NewWithT(t)
		err := &apierrors.StatusError{ErrStatus: metav1.Status{
			Status: metav1.StatusFailure,
			Reason: metav1.StatusReasonStoreReadError,
		}}
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryServer)))
		g.Expect(result).To(HaveField("Code", Equal("STORE_READ_ERROR")))
		g.Expect(result).To(HaveField("Retriable", BeTrue()))
	})

	t.Run("should classify wrapped kubernetes errors", func(t *testing.T) {
		g := NewWithT(t)
		original := apierrors.NewUnauthorized("expired")
		wrapped := fmt.Errorf("failed to create REST config: %w", original)
		result := clierrors.Classify(wrapped)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryAuthentication)))
		g.Expect(result).To(HaveField("Code", Equal("AUTH_FAILED")))
	})

	t.Run("should classify network timeout", func(t *testing.T) {
		g := NewWithT(t)
		err := &testNetError{timeout: true}
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryTimeout)))
		g.Expect(result).To(HaveField("Code", Equal("NET_TIMEOUT")))
		g.Expect(result).To(HaveField("Retriable", BeTrue()))
	})

	t.Run("should classify network error as connection", func(t *testing.T) {
		g := NewWithT(t)
		err := &testNetError{timeout: false}
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryConnection)))
		g.Expect(result).To(HaveField("Code", Equal("CONN_FAILED")))
		g.Expect(result).To(HaveField("Retriable", BeTrue()))
	})

	t.Run("should classify deadline exceeded as timeout", func(t *testing.T) {
		g := NewWithT(t)
		result := clierrors.Classify(context.DeadlineExceeded)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryTimeout)))
		g.Expect(result).To(HaveField("Code", Equal("TIMEOUT")))
		g.Expect(result).To(HaveField("Retriable", BeTrue()))
	})

	t.Run("should classify canceled as internal", func(t *testing.T) {
		g := NewWithT(t)
		result := clierrors.Classify(context.Canceled)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryInternal)))
		g.Expect(result).To(HaveField("Code", Equal("CANCELED")))
		g.Expect(result).To(HaveField("Retriable", BeFalse()))
	})

	t.Run("should classify filesystem path error as validation", func(t *testing.T) {
		g := NewWithT(t)
		err := &fs.PathError{Op: "stat", Path: "not/a/real/file", Err: fs.ErrNotExist}
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryValidation)))
		g.Expect(result).To(HaveField("Code", Equal("CONFIG_INVALID")))
		g.Expect(result).To(HaveField("Retriable", BeFalse()))
		g.Expect(result).To(HaveField("Suggestion", Not(BeEmpty())))
	})

	t.Run("should classify wrapped filesystem error as validation", func(t *testing.T) {
		g := NewWithT(t)
		pathErr := &fs.PathError{Op: "stat", Path: "/bad/kubeconfig", Err: fs.ErrNotExist}
		wrapped := fmt.Errorf("failed to create REST config: %w", pathErr)
		result := clierrors.Classify(wrapped)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryValidation)))
		g.Expect(result).To(HaveField("Code", Equal("CONFIG_INVALID")))
	})

	t.Run("should classify config error as validation", func(t *testing.T) {
		g := NewWithT(t)
		err := clierrors.NewConfigError(errors.New(`context "fake" does not exist`))
		result := clierrors.Classify(err)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryValidation)))
		g.Expect(result).To(HaveField("Code", Equal("CONFIG_INVALID")))
		g.Expect(result).To(HaveField("Retriable", BeFalse()))
		g.Expect(result).To(HaveField("Suggestion", ContainSubstring("kubeconfig")))
	})

	t.Run("should classify wrapped config error as validation", func(t *testing.T) {
		g := NewWithT(t)
		cfgErr := clierrors.NewConfigError(errors.New(`cluster "fake" does not exist`))
		wrapped := fmt.Errorf("failed to create REST config: %w", cfgErr)
		result := clierrors.Classify(wrapped)

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryValidation)))
		g.Expect(result).To(HaveField("Code", Equal("CONFIG_INVALID")))
	})

	t.Run("should classify unknown error as internal", func(t *testing.T) {
		g := NewWithT(t)
		result := clierrors.Classify(errors.New("something unexpected"))

		g.Expect(result).To(HaveField("Category", Equal(clierrors.CategoryInternal)))
		g.Expect(result).To(HaveField("Code", Equal("INTERNAL")))
		g.Expect(result).To(HaveField("Retriable", BeFalse()))
	})

	t.Run("should return nil for nil error", func(t *testing.T) {
		g := NewWithT(t)
		result := clierrors.Classify(nil)

		g.Expect(result).ToNot(HaveOccurred())
	})

	t.Run("should implement error interface", func(t *testing.T) {
		g := NewWithT(t)
		se := &clierrors.StructuredError{Category: clierrors.CategoryAuthentication, Message: "test"}

		g.Expect(se.Error()).To(Equal("[authentication] test"))
	})

	t.Run("should preserve error chain in NewAlreadyHandledError", func(t *testing.T) {
		g := NewWithT(t)
		original := errors.New("token expired")
		wrapped := clierrors.NewAlreadyHandledError(original)

		g.Expect(wrapped).To(MatchError(ContainSubstring("token expired")))
		g.Expect(errors.Is(wrapped, clierrors.ErrAlreadyHandled)).To(BeTrue())
		g.Expect(errors.Is(wrapped, original)).To(BeTrue())
	})
}
