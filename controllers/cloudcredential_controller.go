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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	RBAC_API_GROUP      = "rbac.authorization.k8s.io"
	AWS_CREDENTIAL_PATH = "/root/.aws"
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
	r.createDeployment(cloudCredential)
	//r.createService(cloudCredential)
	r.changeToStar(cloudCredential)

	// TODO
	/*
	   - service 생성
	   - secret 존재할시 덮어쓰기 등 처리
	   - secret - crd 동기화 문제 (삭제됐을 때 어떻게 할지...)(replica?)
	   - secret에 대한 권한 Role을 사전생성
	   - owner annotation 달아줘야함
	*/
	// BUG
	/*
		- rolebinding 안 생김
		- secret 안에 두개의 파일 분리해서 마운트해야함..? (keyPath)
		- role의 resource가 이상하게 됨
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
	log.Info("Create Secret For " + cc.Name + " owner Start")
	secretFound := &corev1.Secret{}

	if err := r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-credential", Namespace: cc.Namespace}, secretFound); err != nil && errors.IsNotFound(err) {
		ccSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        cc.Name + "-credential",
				Namespace:   cc.Namespace,
				Annotations: cc.Annotations,
			},
			//Immutable: false,
			Type:       corev1.SecretTypeOpaque,
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
	log.Info("Create Role For " + cc.Name + " owner Start")
	roleFound := &rbacApi.Role{}

	if err := r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-owner", Namespace: cc.Namespace}, roleFound); err != nil && errors.IsNotFound(err) {
		ccRole := &rbacApi.Role{
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

		if err := r.Create(context.TODO(), ccRole); err != nil && errors.IsNotFound(err) {
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
	log.Info("Create Rolebinding For " + cc.Name + " owner Start")
	rbFound := &rbacApi.RoleBinding{}

	if err := r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-owner", Namespace: cc.Namespace}, rbFound); err != nil && errors.IsNotFound(err) {
		ccRoleBinding := &rbacApi.RoleBinding{
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
			RoleRef: rbacApi.RoleRef{
				Kind:     "Role",
				APIGroup: RBAC_API_GROUP,
				Name:     cc.Name + "-owner",
			},
		}

		if err := r.Create(context.TODO(), ccRoleBinding); err != nil && errors.IsNotFound(err) {
			log.Info("RoleBinding for CloudCredential [ " + cc.Name + " ] Already Exists")
		} else {
			log.Info("Create RoleBinding [ " + cc.Name + "-owner ] Success")
		}

	} else {
		log.Info("Rolebinding for CloudCredential [ " + cc.Name + " ] Already Exists")
	}
}

func (r *CloudCredentialReconciler) createDeployment(cc *credential.CloudCredential) {
	log := r.Log
	log.Info("Create Deployment For " + cc.Name + " owner Start")
	deployFound := &appsv1.Deployment{}

	if err := r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-credential-server", Namespace: cc.Namespace}, deployFound); err != nil && errors.IsNotFound(err) {
		ccDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:        cc.Name + "-credential-server",
				Namespace:   cc.Namespace,
				Labels:      cc.Labels,
				Annotations: cc.Annotations,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"hypercloud": cc.Name + "-credential-server",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"hypercloud": cc.Name + "-credential-server",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "credential-server",
								Image: "192.168.9.12:5000/cc-light-api-server:v0.0.0", // should be modified
								Ports: []corev1.ContainerPort{
									{
										Name:          "http",
										Protocol:      corev1.ProtocolTCP,
										ContainerPort: 80,
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "credential",
										MountPath: AWS_CREDENTIAL_PATH,
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "credential",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: cc.Name + "-credential",
									},
								},
							},
						},
					},
				},
			},
		}

		if err := r.Create(context.TODO(), ccDeployment); err != nil && errors.IsNotFound(err) {
			log.Info("Deployment for CloudCredential [ " + cc.Name + " ] Already Exists")
		} else {
			log.Info("Create Deployment [ " + cc.Name + "-credential-server ] Success")
		}

	} else {
		log.Info("Deployment for CloudCredential [ " + cc.Name + " ] Already Exists")
	}
}

func int32Ptr(i int32) *int32 { return &i }
