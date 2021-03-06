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
	"os"
	"regexp"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacApi "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	credential "github.com/tmax-cloud/cloud-credential-operator/api/v1alpha1"
)

// TODO
/*
		- validation 웹훅 (아니면 그냥 required 필드로 막던가)
		- 경량 서버 직접 접근 인가 로직 (우선순위 나중에...) -> clusterIP도 none으로 할 거라 필요 없을듯?
	   	- secret 존재할시 덮어쓰기 등 처리 -> 애초에 이름 중복이면 안 만들어짐
	   	- secret - crd 동기화 문제 (삭제됐을 때 어떻게 할지...) -> 지우고 새로 만드는 방향으로 유도
*/
// BUG
/*
 */

const (
	RBAC_API_GROUP = "rbac.authorization.k8s.io"
	TMAX_API_GROUP = "credentials.tmax.io"

	AWS_CREDENTIAL_PATH = "/root/.aws"
	AWS_IMAGE_REPO      = "tmaxcloudck/cloud-credential-api-server"

	GCP_CREDENTIAL_PATH = "/root/"        // modify later
	GCP_IMAGE_REPO      = "example-image" // modify later
)

// CloudCredentialReconciler reconciles a CloudCredential object
type CloudCredentialReconciler struct {
	client.Client
	Log         logr.Logger
	Scheme      *runtime.Scheme
	patchHelper *patch.Helper
}

var (
	cc_labels     map[string]string
	AWS_IMAGE_TAG string
	GCP_IMAGE_TAG string
)

func init() {
	AWS_IMAGE_TAG = envFrom(AWS_IMAGE_TAG, "5.0.0.0")
	GCP_IMAGE_TAG = envFrom(GCP_IMAGE_TAG, "5.0.0.0")
}

func envFrom(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// +kubebuilder:rbac:groups=credentials.tmax.io,resources=cloudcredentials,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=credentials.tmax.io,resources=cloudcredentials/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=*,resources=*,verbs=*

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

	if helper, err := patch.NewHelper(cloudCredential, r.Client); err != nil {
		return ctrl.Result{}, err
	} else {
		r.patchHelper = helper
	}
	defer func() {
		r.patchHelper.Patch(context.TODO(), cloudCredential)
	}()

	log.Info("Resource Name [ " + cloudCredential.Name + " ]")
	log.Info("Resource Status : " + cloudCredential.Status.Status)

	switch cloudCredential.Status.Status {
	case "":
		// Set Owner Annotation from Annotation 'Creator'
		if cloudCredential.Annotations != nil && cloudCredential.Annotations["creator"] != "" && cloudCredential.Annotations["owner"] == "" {
			log.Info("Set Owner Annotation from Annotation 'Creator'")
			cloudCredential.Annotations["owner"] = cloudCredential.Annotations["creator"]
		}

		ccList := &credential.CloudCredentialList{}
		if err = r.List(context.TODO(), ccList); err != nil {
			log.Error(err, "Failed to get CloudCredential List")
			panic(err)
		}

		// Check Duplicated Name
		duplicated := false
		for _, cc := range ccList.Items {
			if cc.Namespace == cloudCredential.Namespace && cc.Name == cloudCredential.Name &&
				cc.CreationTimestamp != cloudCredential.CreationTimestamp {
				duplicated = true
				break
			}
		}

		if !duplicated {
			log.Info("CloudCredential [ " + cloudCredential.Name + " ] Not found.")
			cloudCredential.Status.Status = credential.CloudCredentialStatusTypeAwaiting
			cloudCredential.Status.Reason = "Please Wait for Creating required resources"
		} else {
			log.Info("CloudCredential [ " + cloudCredential.Name + " ] Already Exists.")
			cloudCredential.Status.Status = credential.CloudCredentialStatusTypeError
			cloudCredential.Status.Reason = "Duplicated Reosurce Name"
		}
	//case credential.CloudCredentialStatusTypeCreated:
	//case credential.CloudCredentialStatusTypeError:
	case credential.CloudCredentialStatusTypeAwaiting:
		fallthrough // 디버깅을 위해 awaiting 상태에서도 생성 시키기 위해...
	case credential.CloudCredentialStatusTypeApproved:
		var data map[string]string
		data = make(map[string]string)
		cc_labels = make(map[string]string)
		if cloudCredential.Labels != nil {
			cc_labels = cloudCredential.Labels
		}
		cc_labels["fromCloudCredential"] = cloudCredential.Name

		switch cloudCredential.Provider {
		case "aws", "AWS":
			for _, spec := range cloudCredential.Spec {
				profile := spec.Profile
				accessKeyID := spec.AccessKeyID
				accessKey := spec.AccessKey
				region := spec.Region

				data["credentials"] += "[" + profile + "]\n" + "aws_access_key_id = " + accessKeyID + "\n" + "aws_secret_access_key = " + accessKey + "\n"
				data["config"] += "[" + profile + "]\n" + "region = " + region + "\n" + "output = json" + "\n"
			}
		case "gcp", "GCP":
		}

		// secret, role, rolebinding, deployment, service 생성
		if err := r.createResource(cloudCredential, data); err != nil {
			cloudCredential.Status.Status = credential.CloudCredentialStatusTypeError
			cloudCredential.Status.Message = err.Error()
			return ctrl.Result{}, err
		}
		r.changeToStar(cloudCredential)
		cloudCredential.Status.Reason = "Successfully Created"
	case credential.CloudCredentialStatusTypeDeleted:
		if err := r.deleteResource(cloudCredential); err != nil {
			cloudCredential.Status.Status = credential.CloudCredentialStatusTypeError
			cloudCredential.Status.Message = err.Error()
			return ctrl.Result{}, err
		}
		cloudCredential.Status.Reason = "All Resources Deleted"
	}
	return ctrl.Result{}, nil
}

func (r *CloudCredentialReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&credential.CloudCredential{}).
		Complete(r)
}

func (r *CloudCredentialReconciler) createResource(cc *credential.CloudCredential, data map[string]string) error {
	log := r.Log
	log.Info("Create Resource For " + cc.Name + " Start")
	if err := r.createSecret(cc, data); err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to create Secret")
		cc.Status.Reason = "Failed to create Secret"
		return err
	}
	if err := r.createRole(cc); err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to create Role")
		cc.Status.Reason = "Failed to create Role"
		return err
	}
	if err := r.createRoleBinding(cc); err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to create RoleBinding")
		cc.Status.Reason = "Failed to create RoleBinding"
		return err
	}
	if err := r.createDeployment(cc); err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to create Deployment")
		cc.Status.Reason = "Failed to create Deployment"
		return err
	}
	if err := r.createService(cc); err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to create Service")
		cc.Status.Reason = "Failed to create Service"
		return err
	}
	return nil
}

func (r *CloudCredentialReconciler) deleteResource(cc *credential.CloudCredential) error {
	var err, reason error
	log := r.Log
	log.Info("Delete Resource For " + cc.Name + " Start")
	serviceFound := &corev1.Service{}
	if err = r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-credential-server-service", Namespace: cc.Namespace}, serviceFound); err != nil && errors.IsNotFound(err) {
		log.Info("Service  [ " + cc.Name + "-credential-server-service ] Not Exists")
	} else if err != nil {
		log.Error(err, "Failed to get Service [ "+cc.Name+"-credential-server-service ]")
		reason = err
	} else {
		if err = r.Delete(context.TODO(), serviceFound); err != nil {
			log.Error(err, "Failed to delete Service [ "+cc.Name+"-credential-server-service ]")
			reason = err
		} else {
			log.Info("Delete Service  [ " + cc.Name + "-credential-server-service ] Success")
		}
	}

	deployFound := &appsv1.Deployment{}
	if err = r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-credential-server", Namespace: cc.Namespace}, deployFound); err != nil && errors.IsNotFound(err) {
		log.Info("Deployment  [ " + cc.Name + "-credential-server ] Not Exists")
	} else if err != nil {
		log.Error(err, "Failed to get Deployment [ "+cc.Name+"-credential-server ]")
		reason = err
	} else {
		if err = r.Delete(context.TODO(), deployFound); err != nil {
			log.Error(err, "Failed to delete Deployment [ "+cc.Name+"-credential-server ]")
			reason = err
		} else {
			log.Info("Delete Deployment  [ " + cc.Name + "-credential-server ] Success")
		}
	}

	rbFound := &rbacApi.RoleBinding{}
	if err = r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-owner", Namespace: cc.Namespace}, rbFound); err != nil && errors.IsNotFound(err) {
		log.Info("RoleBinding  [ " + cc.Name + "-owner ] Not Exists")
	} else if err != nil {
		log.Error(err, "Failed to get RoleBinding [ "+cc.Name+"-owner ]")
		reason = err
	} else {
		if err = r.Delete(context.TODO(), rbFound); err != nil {
			log.Error(err, "Failed to delete RoleBinding [ "+cc.Name+"-owner ]")
			reason = err
		} else {
			log.Info("Delete RoleBinding  [ " + cc.Name + "-owner ] Success")
		}
	}

	roleFound := &rbacApi.Role{}
	if err = r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-owner", Namespace: cc.Namespace}, roleFound); err != nil && errors.IsNotFound(err) {
		log.Info("Role  [ " + cc.Name + "-owner ] Not Exists")
	} else if err != nil {
		log.Error(err, "Failed to get Role [ "+cc.Name+"-owner ]")
		reason = err
	} else {
		if err = r.Delete(context.TODO(), roleFound); err != nil {
			log.Error(err, "Failed to delete Role [ "+cc.Name+"-owner ]")
			reason = err
		} else {
			log.Info("Delete Role  [ " + cc.Name + "-owner ] Success")
		}
	}

	secretFound := &corev1.Secret{}
	if err = r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-credential", Namespace: cc.Namespace}, secretFound); err != nil && errors.IsNotFound(err) {
		log.Info("Secret  [ " + cc.Name + "-credential ] Not Exists")
	} else if err != nil {
		log.Error(err, "Failed to get Secret [ "+cc.Name+"-credential ]")
		reason = err
	} else {
		if err = r.Delete(context.TODO(), secretFound); err != nil {
			log.Error(err, "Failed to delete Secret [ "+cc.Name+"-credential ]")
			reason = err
		} else {
			log.Info("Delete Secret  [ " + cc.Name + "-credential ] Success")
		}
	}

	return reason
}

func (r *CloudCredentialReconciler) changeToStar(cc *credential.CloudCredential) {
	m1 := regexp.MustCompile(`.`)
	m2 := regexp.MustCompile(`.*`)

	for _, spec := range cc.Spec {
		spec.AccessKeyID = m1.ReplaceAllString(spec.AccessKeyID, "*")
		spec.AccessKey = m2.ReplaceAllString(spec.AccessKey, "Stored in Secret")
	}
}

func (r *CloudCredentialReconciler) createSecret(cc *credential.CloudCredential, data map[string]string) error {
	var err error
	log := r.Log
	log.Info("Create Secret [ " + cc.Name + "-credential ] Start")
	secretFound := &corev1.Secret{}

	if err = r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-credential", Namespace: cc.Namespace}, secretFound); err != nil && errors.IsNotFound(err) {
		ccSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        cc.Name + "-credential",
				Namespace:   cc.Namespace,
				Annotations: cc.Annotations,
				Labels:      cc_labels,
			},
			//Immutable: false,
			Type:       corev1.SecretTypeOpaque,
			StringData: data,
		}

		if err = r.Create(context.TODO(), ccSecret); err != nil && errors.IsNotFound(err) {
			log.Info("Secret [ " + cc.Name + "-credential ] Already Exists")
		} else {
			log.Info("Create Secret [ " + cc.Name + "-credential ] Success")
		}
	} else {
		log.Info("Secret [ " + cc.Name + "-credential ] Already Exists")
	}
	return err
}

func (r *CloudCredentialReconciler) createRole(cc *credential.CloudCredential) error {
	var err error
	log := r.Log
	log.Info("Create Role [ " + cc.Name + "-owner ] Start")
	roleFound := &rbacApi.Role{}

	if err = r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-owner", Namespace: cc.Namespace}, roleFound); err != nil && errors.IsNotFound(err) {
		ccRole := &rbacApi.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:        cc.Name + "-owner",
				Namespace:   cc.Namespace,
				Labels:      cc_labels,
				Annotations: cc.Annotations,
			},
			Rules: []rbacApi.PolicyRule{
				{
					APIGroups: []string{
						TMAX_API_GROUP,
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

		if err = r.Create(context.TODO(), ccRole); err != nil && errors.IsNotFound(err) {
			log.Info("Role [ " + cc.Name + "-owner ] Already Exists")
		} else if err != nil {
			log.Error(err, "Failed to create Role [ "+cc.Name+"-owner ]")
		} else {
			log.Info("Role [ " + cc.Name + "-owner ] Success")
		}
	} else {
		log.Info("Role [ " + cc.Name + "-owner ] Already Exists")
	}
	return err
}

func (r *CloudCredentialReconciler) createRoleBinding(cc *credential.CloudCredential) error {
	var err error
	log := r.Log
	log.Info("Create Rolebinding [ " + cc.Name + "-owner ] Start")
	rbFound := &rbacApi.RoleBinding{}

	if err = r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-owner", Namespace: cc.Namespace}, rbFound); err != nil && errors.IsNotFound(err) {
		ccRoleBinding := &rbacApi.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:        cc.Name + "-owner",
				Namespace:   cc.Namespace,
				Labels:      cc_labels,
				Annotations: cc.Annotations,
			},
			Subjects: []rbacApi.Subject{
				{
					Name:     cc.Annotations["owner"],
					Kind:     "User",
					APIGroup: RBAC_API_GROUP,
				},
			},
			RoleRef: rbacApi.RoleRef{
				Kind:     "Role",
				APIGroup: RBAC_API_GROUP,
				Name:     cc.Name + "-owner",
			},
		}

		if err = r.Create(context.TODO(), ccRoleBinding); err != nil && errors.IsNotFound(err) {
			log.Info("RoleBinding [ " + cc.Name + "-owner ] Already Exists")
		} else if err != nil {
			log.Error(err, "Failed to create Rolebinding")
		} else {
			log.Info("Create RoleBinding [ " + cc.Name + "-owner ] Success")
		}

	} else {
		log.Info("Rolebinding [ " + cc.Name + "-owner ] Already Exists")
	}
	return err
}

func (r *CloudCredentialReconciler) createDeployment(cc *credential.CloudCredential) error {
	var err error
	log := r.Log
	log.Info("Create Deployment [ " + cc.Name + "-credential-server ] Start")

	var CREDENTIAL_PATH string
	switch cc.Provider {
	case "aws", "AWS":
		CREDENTIAL_PATH = AWS_CREDENTIAL_PATH
	case "gcp", "GCP":
		CREDENTIAL_PATH = GCP_CREDENTIAL_PATH
	}

	deployFound := &appsv1.Deployment{}
	if err = r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-credential-server", Namespace: cc.Namespace}, deployFound); err != nil && errors.IsNotFound(err) {
		ccDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:        cc.Name + "-credential-server",
				Namespace:   cc.Namespace,
				Labels:      cc_labels,
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
								Image: AWS_IMAGE_REPO + ":b" + AWS_IMAGE_TAG,
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
										MountPath: CREDENTIAL_PATH,
									},
								},
								Resources: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										"cpu":    resource.MustParse("100m"),
										"memory": resource.MustParse("30Mi"),
									},
									// Requests: corev1.ResourceList{
									// 	"cpu":    resource.MustParse("100m"),
									// 	"memory": resource.MustParse("20Mi"),
									// },
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

		if err = r.Create(context.TODO(), ccDeployment); err != nil && errors.IsNotFound(err) {
			log.Info("Deployment [ " + cc.Name + "-credential-server ] Already Exists")
		} else {
			log.Info("Create Deployment [ " + cc.Name + "-credential-server ] Success")
		}

	} else {
		log.Info("Deployment [ " + cc.Name + "-credential-server ] Already Exists")
	}
	return err
}

func (r *CloudCredentialReconciler) createService(cc *credential.CloudCredential) error {
	var err error
	log := r.Log
	log.Info("Create Service [ " + cc.Name + "-credential-server-service ] Start")
	serviceFound := &corev1.Service{}

	if err = r.Get(context.TODO(), types.NamespacedName{Name: cc.Name + "-credential-server-service", Namespace: cc.Namespace}, serviceFound); err != nil && errors.IsNotFound(err) {
		ccService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        cc.Name + "-credential-server-service",
				Namespace:   cc.Namespace,
				Labels:      cc_labels,
				Annotations: cc.Annotations,
			},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeClusterIP,
				ClusterIP: "None",
				Selector: map[string]string{
					"hypercloud": cc.Name + "-credential-server",
				},
				Ports: []corev1.ServicePort{
					{
						Port: 80,
					},
				},
			},
		}
		if err = r.Create(context.TODO(), ccService); err != nil && errors.IsNotFound(err) {
			log.Info("Service [ " + cc.Name + "-credential-server-service ] Already Exists")
		} else {
			log.Info("Create Service [ " + cc.Name + "-credential-server-service ] Success")
		}
	} else {
		log.Info("Service [ " + cc.Name + "-credential-server-service ] Already Exists")
	}
	return err
}
func int32Ptr(i int32) *int32 { return &i }
