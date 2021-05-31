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
	rbacApi "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	credential "github.com/tmax-cloud/cloud-credential-operator/api/v1alpha1"
)

const (
	RBAC_API_GROUP = "rbac.authorization.k8s.io"
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

	r.createSecret(cloudCredential, data)
	r.createRole(cloudCredential)
	r.createRoleBinding(cloudCredential)
	r.changeToStar(cloudCredential)

	// TODO
	/*
	   - secret 존재할시 덮어쓰기 등 처리
	   - secret - crd 동기화 문제 (삭제됐을 때 어떻게 할지...)(replica?)

	   - secret에 대한 권한 Role을 사전생성
	   - owner annotaiong 달아줘야함
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

func (r *CloudCredentialReconciler) createSecret(cc *credential.CloudCredential, data map[string]string) {
	log := r.Log
	log.Info("Create Secret For CloudCredential owner Start")
	secretFound := &v1.Secret{}

	if err := r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-credential", Namespace: cc.Namespace}, secretFound); err != nil && errors.IsNotFound(err) {
		ccSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        cc.Name + "-credential",
				Namespace:   cc.Namespace,
				Annotations: cc.Annotations,
			},
			//Immutable: false,
			Type:       v1.SecretTypeOpaque,
			StringData: data,
		}

		if err := r.Create(context.TODO(), ccSecret); err != nil && errors.IsNotFound(err) {
			log.Info("RoleBinding for CloudCredential [ " + cc.Name + " ] Already Exists")
		} else {
			log.Info("Successfully create Secret")
		}
	} else {
		log.Info("RoleBinding for CloudCredential [ " + cc.Name + " ] Already Exists")
	}
}

func (r *CloudCredentialReconciler) createRole(cc *credential.CloudCredential) {
	log := r.Log
	log.Info("Create Role For CloudCredential owner Start")
	roleFound := &rbacApi.Role{}

	if err := r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-owner", Namespace: cc.Namespace}, roleFound); err != nil && errors.IsNotFound(err) {
		roleForCCOwner := &rbacApi.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:        cc.Name + "-owner",
				Namespace:   cc.Namespace,
				Labels:      cc.Labels,
				Annotations: cc.Annotations,
			},
			Rules: []rbacApi.PolicyRule{
				{
					APIGroups: []string{
						RBAC_API_GROUP,
					},
					Resources: []string{
						"cloudcredential",
					},
					ResourceNames: []string{
						cc.Name,
					},
					Verbs: []string{
						"get",
						"update",
						"patch",
						"delete",
					},
				},
			},
		}

		if err := r.Create(context.TODO(), roleForCCOwner); err != nil && errors.IsNotFound(err) {
			log.Info("Role for CloudCredential [ " + cc.Name + " ] Already Exists")
		} else {
			log.Info("Create Role [ " + cc.Name + "-owner ] Success")
		}

	} else {
		log.Info("Roleb for CloudCredential [ " + cc.Name + " ] Already Exists")
	}
}

func (r *CloudCredentialReconciler) createRoleBinding(cc *credential.CloudCredential) {
	log := r.Log
	log.Info("Create Rolebinding For CloudCredential owner Start")
	rbFound := &rbacApi.RoleBinding{}

	if err := r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-owner", Namespace: cc.Namespace}, rbFound); err != nil && errors.IsNotFound(err) {
		rbForCCOwner := &rbacApi.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:        cc.Name + "-owner",
				Namespace:   cc.Namespace,
				Labels:      cc.Labels,
				Annotations: cc.Annotations,
			},
			Subjects: []rbacApi.Subject{
				{
					Kind:     "User",
					APIGroup: RBAC_API_GROUP,
					Name:     cc.Annotations["owner"],
				},
			},
			RoleRef: rbacApi.RoleRef{ // 실제 생성해줘야함
				Kind:     "Role",
				APIGroup: RBAC_API_GROUP,
				Name:     cc.Name + "-owner",
			},
		}

		if err := r.Create(context.TODO(), rbForCCOwner); err != nil && errors.IsNotFound(err) {
			log.Info("RoleBinding for CloudCredential [ " + cc.Name + " ] Already Exists")
		} else {
			log.Info("Create RoleBinding [ " + cc.Name + "-owner ] Success")
		}

	} else {
		log.Info("Rolebinding for CloudCredential [ " + cc.Name + " ] Already Exists")
	}
}
