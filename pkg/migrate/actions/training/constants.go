package training

const (
	verifyWorkloadsID          = "training.verify-workloads"
	verifyWorkloadsName        = "Verify training workloads"
	verifyWorkloadsDescription = "Pre-upgrade check for Kubeflow v1 training workloads requiring migration to Trainer v2 TrainJob"
)

//nolint:gochecknoglobals // Immutable mapping from v1 training job kinds to v2 TrainJob runtimes
var v1KindToV2Runtime = map[string]string{
	"PyTorchJob": "torch",
	"TFJob":      "tensorflow",
	"MPIJob":     "mpi",
	"XGBoostJob": "xgboost",
}
