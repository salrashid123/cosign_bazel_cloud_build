package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"time"

	//"github.com/google/go-containerregistry/pkg/authn"

	"github.com/go-openapi/runtime"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	cosign "github.com/sigstore/cosign/pkg/cosign"
	ociremote "github.com/sigstore/cosign/pkg/oci/remote"
	sigs "github.com/sigstore/cosign/pkg/signature"
	rekor "github.com/sigstore/rekor/pkg/client"
	"github.com/sigstore/rekor/pkg/generated/client/entries"
	"github.com/sigstore/rekor/pkg/generated/client/index"
	"github.com/sigstore/rekor/pkg/generated/models"
	"github.com/sigstore/rekor/pkg/types"
	"github.com/sigstore/sigstore/pkg/signature/payload"
	"golang.org/x/oauth2/google"
)

var (
	hash     = flag.String("hash", "a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00", "Raw image hash value")
	imageRef = flag.String("imageRef", "us-central1-docker.pkg.dev/YOUR_PROJECT_ID_HERE/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00", "Image Referenc")
	kmspub   = flag.String("kmspub", "../kms_pub.pem", "KMS Public Key")
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

	fmt.Println(">>>>>>>>>> Search rekor <<<<<<<<<<")

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
			fmt.Printf(" UUID %s\n", *en.LogID)

			// todo find out ow to extract the intoto as go objects from body
			// https://github.com/sigstore/rekor/tree/main/pkg/types
			// https://github.com/sigstore/rekor/blob/main/pkg/types/intoto/v0.0.1/entry.go

			b, err := base64.StdEncoding.DecodeString(en.Body.(string))
			if err != nil {
				panic(err)
			}

			pe, err := models.UnmarshalProposedEntry(bytes.NewReader(b), runtime.JSONConsumer())
			if err != nil {
				panic(err)
			}
			eimpl, err := types.UnmarshalEntry(pe)
			if err != nil {
				panic(err)
			}

			fmt.Printf(" Entry API Version %s\n", eimpl.APIVersion())

			// just parse inttoto types since thats what we submitted
			it, ok := pe.(*models.Intoto)
			if !ok {
				panic(err)
			}
			fmt.Printf(" Kind: %s\n", it.Kind())

			var ta models.IntotoV001Schema
			if err := types.DecodeEntry(it.Spec, &ta); err != nil {
				panic(err)
			}

			dec, err := base64.RawStdEncoding.DecodeString(ta.PublicKey.String())
			if err := types.DecodeEntry(it.Spec, &ta); err != nil {
				panic(err)
			}
			if err := types.DecodeEntry(it.Spec, &ta); err != nil {
				panic(err)
			}
			fmt.Printf(" PublicKey:\n%s\n", dec)

			// this is just to demo:  it verifies an entry is included in the tlog...though we just
			// got the entry from the tlog in the first place...
			err = cosign.VerifyTLogEntry(ctx, rekorClient, &en)
			if err != nil {
				panic(err)
			}
			fmt.Printf(" rekor logentry inclustion verified\n")
		}
	}

	// *******************

	fmt.Println(">>>>>>>>>> Verifying Image Signatures using provided PublicKey <<<<<<<<<<")
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
	ts, _ := google.DefaultTokenSource(ctx)
	if err != nil {
		panic(err)
	}
	t, err := ts.Token()
	if err != nil {
		panic(err)
	}

	opts := []remote.Option{
		remote.WithAuth(&authn.Bearer{
			Token: t.AccessToken,
		}),
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
		fmt.Println("Bundle Verified")
	}
	for _, s := range c {
		bsig, err := s.Base64Signature()
		if err != nil {
			panic(err)
		}
		fmt.Printf("Verified signature %s\n", bsig)

		p, err := s.Payload()
		if err != nil {
			panic(err)
		}
		ss := payload.SimpleContainerImage{}
		if err := json.Unmarshal(p, &ss); err != nil {
			fmt.Println("error decoding the payload:", err.Error())
			return
		}
		fmt.Printf("  Image Ref %s\n", ss.Critical.Image)

	}

}
