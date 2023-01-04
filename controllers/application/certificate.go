package applicationcontroller

import (
	"context"
	"fmt"
	"regexp"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	skiperatorv1alpha1 "github.com/kartverket/skiperator/api/v1alpha1"
	util "github.com/kartverket/skiperator/pkg/util"
	"golang.org/x/exp/slices"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ApplicationReconciler) SkiperatorOwnedCertRequests(obj client.Object) []reconcile.Request {
	certificate, isCert := obj.(*certmanagerv1.Certificate)

	if !isCert {
		return nil
	}

	isSkiperatorOwned := certificate.Labels["app.kubernetes.io/managed-by"] == "skiperator"

	requests := make([]reconcile.Request, 0)

	if isSkiperatorOwned {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: certificate.Labels["application.skiperator.no/app-namespace"],
				Name:      certificate.Labels["application.skiperator.no/app-name"],
			},
		})
	}

	return requests
}

func (r *ApplicationReconciler) reconcileCertificate(ctx context.Context, application *skiperatorv1alpha1.Application) (reconcile.Result, error) {
	controllerName := "Certificate"
	r.SetControllerProgressing(ctx, application, controllerName)

	// Generate separate gateway for each ingress
	for _, hostname := range application.Spec.Ingresses {
		name := fmt.Sprintf("%s-%s-ingress-%x", application.Namespace, application.Name, util.GenerateHashFromName(hostname))

		certificate := certmanagerv1.Certificate{ObjectMeta: metav1.ObjectMeta{Namespace: "istio-system", Name: name}}
		_, err := ctrlutil.CreateOrPatch(ctx, r.GetClient(), &certificate, func() error {
			certificate.Spec.IssuerRef.Kind = "ClusterIssuer"
			certificate.Spec.IssuerRef.Name = "cluster-issuer" // Name defined in https://github.com/kartverket/certificate-management/blob/main/clusterissuer.tf
			certificate.Spec.DNSNames = []string{hostname}
			certificate.Spec.SecretName = name

			certLabels := certificate.Labels
			if len(certLabels) == 0 {
				certLabels = make(map[string]string)
			}
			certLabels["app.kubernetes.io/managed-by"] = "skiperator"

			// TODO Find better label names here
			certLabels["application.skiperator.no/app-name"] = application.Name
			certLabels["application.skiperator.no/app-namespace"] = application.Namespace

			certificate.Labels = certLabels

			return nil
		})
		if err != nil {
			r.SetControllerError(ctx, application, controllerName, err)
			return reconcile.Result{}, err
		}
	}

	// Clear out unused certs
	certificates, err := r.GetSkiperatorOwnedCertificates(ctx)

	if err != nil {
		r.SetControllerError(ctx, application, controllerName, err)
		return reconcile.Result{}, err
	}

	for _, certificate := range certificates.Items {

		certificateInApplicationSpecIndex := slices.IndexFunc(application.Spec.Ingresses, func(hostname string) bool {
			certificateName := fmt.Sprintf("%s-%s-ingress-%x", application.Namespace, application.Name, util.GenerateHashFromName(hostname))
			return certificate.Name == certificateName
		})
		certificateInApplicationSpec := certificateInApplicationSpecIndex != -1
		if certificateInApplicationSpec {
			continue
		}

		// We want to delete certificate which are not in the spec, but still "owned" by the application.
		// This should be the case for any certificate not in spec from the earlier continue, if the name still matches <namespace>-<application-name>-ingress-*
		if !r.IsApplicationsCertificate(ctx, *application, certificate) {
			continue
		}

		// Delete the rest
		err = r.GetClient().Delete(ctx, &certificate)
		err = client.IgnoreNotFound(err)
		if err != nil {
			r.SetControllerError(ctx, application, controllerName, err)
			return reconcile.Result{}, err
		}
	}

	r.SetControllerFinishedOutcome(ctx, application, controllerName, err)

	return reconcile.Result{}, err
}

func (r *ApplicationReconciler) GetSkiperatorOwnedCertificates(context context.Context) (certmanagerv1.CertificateList, error) {
	certificates := certmanagerv1.CertificateList{}
	err := r.GetClient().List(context, &certificates, client.MatchingLabels{
		"app.kubernetes.io/managed-by": "skiperator",
	})

	return certificates, err
}

func (r *ApplicationReconciler) IsApplicationsCertificate(context context.Context, application skiperatorv1alpha1.Application, certificate certmanagerv1.Certificate) bool {
	applicationNamespacedName := application.Namespace + "-" + application.Name
	certNameMatchesApplicationNamespacedName, _ := regexp.MatchString("^"+applicationNamespacedName+"-ingress-.+$", certificate.Name)

	return certNameMatchesApplicationNamespacedName
}
