package auth

import (
	"encoding/json"
	"io"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientauthenticationv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
)

func WriteExecCredential(w io.Writer, token TokenSet) error {
	credential := clientauthenticationv1.ExecCredential{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "client.authentication.k8s.io/v1",
			Kind:       "ExecCredential",
		},
		Status: &clientauthenticationv1.ExecCredentialStatus{
			Token:               token.IDToken,
			ExpirationTimestamp: &metav1.Time{Time: token.Expiry},
		},
	}
	encoder := json.NewEncoder(w)
	return encoder.Encode(credential)
}
