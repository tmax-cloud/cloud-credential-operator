/*


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

package controllers

import (
	"context"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	credential "github.com/tmax-cloud/cloud-credential-operator/api/v1alpha1"
)

// CloudCredentialReconciler reconciles a CloudCredential object
type CloudCredentialReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=credentials.tmax.io,resources=cloudcredentials,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=credentials.tmax.io,resources=cloudcredentials/status,verbs=get;update;patch

func (r *CloudCredentialReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	log := r.Log
	log.Info("Reconciling CloudCredential")

	cloudCredential := &credential.CloudCredential{}
	err := r.Get(context.TODO(), req.NamespacedName, cloudCredential)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("CloudCredential resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get CloudCredential")
		return ctrl.Result{}, err
	}

	provider := cloudCredential.Spec.Provider

	accessKeyID := cloudCredential.Spec.AccessKeyID
	accessKey := cloudCredential.Spec.AccessKey
	region := cloudCredential.Spec.Region

	var data map[string]string
	data = make(map[string]string)

	if strings.EqualFold(provider, "aws") {
		data["credentials"] = "[default]\n" + "aws_access_key_id = " + accessKeyID + "\n" + "aws_secret_access_key = " + accessKey + "\n"
		data["config"] = "[default]\n" + "region = " + region + "\n" + "output = json" + "\n"
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cloudCredential.Name,
			Namespace:   cloudCredential.Namespace,
			Annotations: cloudCredential.Annotations,
		},
		//Immutable: false,
		Type:       v1.SecretTypeOpaque,
		StringData: data,
	}

	err = r.Create(context.TODO(), secret)
	if err != nil {
		log.Error(err, "Failed to create Secret")
		cloudCredential.Status.Message = err.Error()
		cloudCredential.Status.Reason = "Failed to create Secret"
	} else {
		log.Info("Successfully create Secret")
		cloudCredential.Status.Reason = "Successfully create Secret"
	}

	r.changeToStar(cloudCredential)

	billing := &credential.Billng{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cloudCredential.Name + "-billing",
			Namespace:   cloudCredential.Namespace,
			Annotations: cloudCredential.Annotations,
		},
		Spec: credential.BillngSpec{
			Provider:       cloudCredential.Spec.Provider,
			CredentialName: cloudCredential.Name,
		},
	}

	err = r.Create(context.TODO(), billing)
	if err != nil {
		log.Error(err, "Failed to create Biliing")
		cloudCredential.Status.Message = err.Error()
		cloudCredential.Status.Reason = "Failed to create Biliing"
	} else {
		log.Info("Successfully create Biliing")
		cloudCredential.Status.Reason = "Successfully create Biliing"
	}

	// TODO
	/*
	   - secret 존재할시 덮어쓰기 등 처리
	   - secret - crd 동기화 문제 (삭제됐을 때 어떻게 할지...)(replica?)
	*/

	return ctrl.Result{}, nil
}

func (r *CloudCredentialReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&credential.CloudCredential{}).
		Complete(r)
}

func (r *CloudCredentialReconciler) changeToStar(cc *credential.CloudCredential) {
	m1 := regexp.MustCompile(`.`)
	cc.Spec.AccessKeyID = m1.ReplaceAllString(cc.Spec.AccessKeyID, "*")

	m2 := regexp.MustCompile(`.*`)
	cc.Spec.AccessKey = m2.ReplaceAllString(cc.Spec.AccessKey, "Stored in Secret")
}
