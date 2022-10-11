package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	//"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	cosign "github.com/sigstore/cosign/pkg/cosign"
	ociremote "github.com/sigstore/cosign/pkg/oci/remote"
	sigs "github.com/sigstore/cosign/pkg/signature"
	rekor "github.com/sigstore/rekor/pkg/client"
	"github.com/sigstore/rekor/pkg/generated/client/entries"
	"github.com/sigstore/rekor/pkg/generated/client/index"
	"github.com/sigstore/rekor/pkg/generated/models"
)

var (
	hash = flag.String("hash", "83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753", "Raw image hash value")
	//imageRef = flag.String("imageRef", "us-central1-docker.pkg.dev/cosign-test-365209/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753", "Image Referenc")
	imageRef = flag.String("imageRef", "docker.io/salrashid123/myimage:server@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753", "Image Reference")
	kmspub   = flag.String("kmspub", "../cert/kms_pub.pem", "KMS Public Key")
)

const ()

func main() {

	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rekorClient, err := rekor.GetRekorClient("https://rekor.sigstore.dev")
	if err != nil {
		panic(err)
	}

	idx, err := rekorClient.Index.SearchIndex(&index.SearchIndexParams{
		Query: &models.SearchIndex{
			Hash: *hash,
		},
		Context: ctx,
	})
	if err != nil {
		panic(err)
	}

	for _, i := range idx.Payload {
		e, err := rekorClient.Entries.GetLogEntryByUUID(&entries.GetLogEntryByUUIDParams{
			EntryUUID: i,
			Context:   ctx,
		})
		if err != nil {
			panic(err)
		}

		for _, en := range e.Payload {
			fmt.Printf("LogIndex %d\n", *en.LogIndex)
			fmt.Printf("UUID %s\n", *en.LogID)
			fmt.Printf("Body %s\n", en.Body)

			err = cosign.VerifyTLogEntry(ctx, rekorClient, &en)
			if err != nil {
				panic(err)
			}
			fmt.Printf("rekor logentry verified\n")
		}
	}

	// *******************
	pubKey, err := sigs.LoadPublicKey(ctx, *kmspub)
	if err != nil {
		panic(err)
	}

	ref, err := name.ParseReference(*imageRef)
	if err != nil {
		panic(err)
	}

	// for artifact registry
	// "github.com/google/go-containerregistry/pkg/authn"
	// "golang.org/x/oauth2/google"
	// ts, _ := google.DefaultTokenSource(ctx)
	// if err != nil {
	// 	panic(err)
	// }
	// t, err := ts.Token()
	// if err != nil {
	// 	panic(err)
	// }

	// opts := []remote.Option{
	// 	remote.WithAuth(&authn.Bearer{
	// 		Token: t.AccessToken,
	// 	}),
	// 	remote.WithContext(ctx),
	// }

	opts := []remote.Option{
		remote.WithContext(ctx),
	}

	co := &cosign.CheckOpts{
		ClaimVerifier:      cosign.SimpleClaimVerifier,
		RegistryClientOpts: []ociremote.Option{ociremote.WithRemoteOptions(opts...)},
		SigVerifier:        pubKey,
	}

	c, ok, err := cosign.VerifyImageSignatures(ctx, ref, co)
	if err != nil {
		panic(err)
	}

	if ok {
		for _, s := range c {
			bsig, err := s.Base64Signature()
			if err != nil {
				panic(err)
			}
			fmt.Printf("Signature %s\n", bsig)

			bun, err := s.Bundle()
			if err != nil {
				panic(err)
			}
			if bun != nil {
				fmt.Printf("   LogIndex %d\n", bun.Payload.LogIndex)

			}

			// p, err := s.Payload()
			// if err != nil {
			// 	panic(err)
			// }
			// fmt.Printf("%v\n", string(p))

		}
	}

}
