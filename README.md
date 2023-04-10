# Deterministic container hashes and container signing using Cosign, Bazel and Google Cloud Build

A simple tutorial that generates consistent container image hashes using `bazel` and then signs provenance records using [cosign](https://github.com/sigstore/cosign) (Container Signing, Verification and Storage in an OCI registry).

In this tutorial, we will:

1. generate a deterministic container image hash using  `bazel`
2. use `cosign` to create provenance records for this image
3. use `syft` to generate the container `sbom`
4. use cosign to sign the container sbom
5. verify attestations and signatures using `KMS` and `OIDC` cross checked with a public transparency log.
6. use `syft` to generate the application `sbom`
7. sign the application `sbom` with cosign

We will use GCP-centric services here such as `Artifact Registry`, `Cloud BUild`, `Cloud Source Repository`.  

Both `KMS` and `OIDC` based signatures are used and for `OIDC`, an entry is submitted to a `transparency log` such that it can get verified by anyone at anytime.

>> **NOTE** Please be aware that if you run this tutorial, the GCP service_accounts _email_ you use to sign the artifacts within cloud build will be submitted to a public transparency log.  I used a disposable GCP project but even if i didn't, its just the email address and projectID in the cert, no big deal to me.  If it is to you, you can use the KMS examples and skip OIDC

>> this repo is not supported by google and employs as much as i know about it on 9/24/22 (with one weeks' experience with this..so take it with a grain of salt)

---

##### References:

* [SigStore](https://docs.sigstore.dev/)
* [cosign](https://github.com/sigstore/cosign)
* [Introducing sigstore: Easy Code Signing & Verification for Supply Chain Integrity](https://security.googleblog.com/2021/03/introducing-sigstore-easy-code-signing.html)
* [Best Practices for Supply Chain Security](https://dlorenc.medium.com/policy-and-attestations-89650fd6f4fa)
* [Building deterministic Docker images with Bazel](https://blog.bazel.build/2015/07/28/docker_build.html#building-deterministic-docker-images-with-bazel)
* [Deterministic builds with go + bazel + grpc + docker](https://github.com/salrashid123/go-grpc-bazel-docker)
* [bazel](https://bazel.build/)
* [in-toto attestation](https://docs.sigstore.dev/cosign/attestation/)
* [Notary V2 and Cosign](https://dlorenc.medium.com/notary-v2-and-cosign-b816658f044d)

### CloudBuild steps

First lets go over the `cloudbuild.yaml` steps:

#### Build image deterministically using bazel:

This is the `bazel` build that guarantees you the code will produce a specific image hash everytime:

* `securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753`

![images/build.png](images/build.png)

#### Push image to registry

This pushes the image to [google artifact registry](https://cloud.google.com/artifact-registry).  This will give the image hash we'd expect

You can optionally push to dockerhub if you want using [KMS based secrets](https://cloud.google.com/build/docs/securing-builds/use-encrypted-credentials#configuring_builds_to_use_encrypted_data) in cloud build 

![images/push.png](images/push.png)


#### Create attestations attributes

This step will issue a statement that includes attestation attributes users can inject into the pipeline the verifier can use. See [Verify Attestations](https://docs.sigstore.dev/cosign/verify/).

In this case, the attestation verification predicate includes some info from the build like the buildID and even the repo commithash.

Someone who wants to verify any [in-toto attestation](https://docs.sigstore.dev/cosign/attestation/) can use these values. This repo just adds some basic stuff like the `projectID`,  `buildID` and `commitsha` (in our case, its `52dc8c1979d7e6b56a5a253dbd79028842752b08`):


```json
{ "projectid": "$PROJECT_ID", "buildid": "$BUILD_ID", "foo":"bar", "commitsha": "$COMMIT_SHA" }
```

![images/attestations.png](images/attestations.png)

against commit

![images/commit.png](images/commit.png)


#### Sign image using KMS based keys

This step uses the KMS key to `cosign` the image

![images/sign_kms.png](images/sign_kms.png)

#### Apply attestations using KMS

This issues attestation signature using some predicates we wrote to file during the build.

You can define any claims here..i just happen to use the commit hash for the source and some random stuff.

![images/attest_kms.png](images/attest_kms.png)

#### Sign image using OIDC tokens

This step will use the service accounts OIDC token sign using [Fulcio](https://docs.sigstore.dev/fulcio/oidc-in-fulcio)

![images/sign_oidc.png](images/sign_kms.png)

#### Apply attestations using OIDC tokens

This will issue signed attestations using the OIDC token signing for fulcio

![images/attest_oidc.png](images/attest_oidc.png)


#### Use Syft to generate image sbom

Generate the container image's sbom

![images/generate_sbom.png](images/generate_sbom.png)


>> **NOTE**  the images i used here will _not_ show the detailed go packages.  see [https://github.com/anchore/syft/issues/1725](https://github.com/anchore/syft/issues/1725)

Then attach (upload) it

![images/attach_sbom.png](images/attach_sbom.png)
#### Use Cosign to upload and sign the sbom

Finally sign it in the registry

![images/sign_sbom.png](images/sign_sbom.png)

---

### Setup

The following steps will use Google Cloud services 

* Cloud Source Repository to hold the code and trigger builds (you can use github but thats out of scope here),
* Cloud Build to create the image to save to artifact registry.
* Artifact Registry to hold the containers images

You'll also need to install [cosign](https://docs.sigstore.dev/cosign/installation/) (duh), and [rekor-cli](https://docs.sigstore.dev/rekor/installation), `git`, `gcloud`, optionally `gcloud`, `docker`.

```bash
export GCLOUD_USER=`gcloud config get-value core/account`
export PROJECT_ID=`gcloud config get-value core/project`
export PROJECT_NUMBER=`gcloud projects describe $PROJECT_ID --format='value(projectNumber)'`
echo $PROJECT_ID

gcloud auth application-default login

# enable services
gcloud services enable \
    artifactregistry.googleapis.com \
    cloudbuild.googleapis.com cloudkms.googleapis.com \
    iam.googleapis.com sourcerepo.googleapis.com

# create artifact registry
gcloud artifacts repositories create repo1 --repository-format=docker --location=us-central1

# create service account that cloud build will run as.
gcloud iam service-accounts create cosign

# allow 'self impersonation' for cloud build service account
gcloud iam service-accounts add-iam-policy-binding cosign@$PROJECT_ID.iam.gserviceaccount.com \
    --role roles/iam.serviceAccountTokenCreator \
    --member "serviceAccount:cosign@$PROJECT_ID.iam.gserviceaccount.com"

# allow cloud build to write logs
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member=serviceAccount:cosign@$PROJECT_ID.iam.gserviceaccount.com  \
  --role=roles/logging.logWriter

# allow cloud build write access to artifact registry
gcloud artifacts repositories add-iam-policy-binding repo1 \
    --location=us-central1  \
    --member=serviceAccount:cosign@$PROJECT_ID.iam.gserviceaccount.com \
    --role=roles/artifactregistry.writer

# allow cloud build access to list KMS keys
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member=serviceAccount:cosign@$PROJECT_ID.iam.gserviceaccount.com  \
  --role=roles/cloudkms.viewer


# create kms keyring and key
gcloud kms keyrings create cosignkr --location=global

gcloud kms keys create key1 --keyring=cosignkr \
 --location=global --purpose=asymmetric-signing \
 --default-algorithm=ec-sign-p256-sha256

gcloud kms keys list  --keyring=cosignkr --location=global

# allow cloud buildaccess to sign the key
gcloud kms keys add-iam-policy-binding key1 \
    --keyring=cosignkr --location=global \
    --member=serviceAccount:cosign@$PROJECT_ID.iam.gserviceaccount.com \
    --role=roles/cloudkms.signer

# allow current gcloud user to view the public key
gcloud kms keys add-iam-policy-binding key1 \
    --keyring=cosignkr --location=global \
    --member=serviceAccount:cosign@$PROJECT_ID.iam.gserviceaccount.com  \
    --role=roles/cloudkms.publicKeyViewer

# create a temp bucket for cloud build and allow cloud build permissions to use it
gsutil mb gs://$PROJECT_ID\_cloudbuild
gsutil iam ch serviceAccount:cosign@$PROJECT_ID.iam.gserviceaccount.com:objectAdmin gs://$PROJECT_ID\_cloudbuild
```

### Build image

```bash
# to build directly
# cd /app
# gcloud beta builds submit --config=cloudbuild.yaml --machine-type=n1-highcpu-32

# to build via commit (recommended)
gcloud source repos create cosign-repo

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member=serviceAccount:cosign@$PROJECT_ID.iam.gserviceaccount.com \
  --role=roles/source.reader

gcloud source repos clone cosign-repo
cd cosign-repo
cp -R ../app/* .
# remember to edit BUILD.bazel and replace the value with the actual project_ID value
# for repository = "us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild"

# optionally create the application sbom and sign it with the same cosign keypair
# goreleaser release --snapshot  --rm-dist 
## for github
## git tag v1.0.0
## git push origin --tags
## goreleaser release --rm-dist


git add -A
git commit -m "add"
git push 

# create a manual trigger
gcloud beta builds triggers create manual --region=global \
   --name=cosign-trigger --build-config=cloudbuild.yaml \
   --repo=https://source.developers.google.com/p/$PROJECT_ID/r/cosign-repo \
   --repo-type=CLOUD_SOURCE_REPOSITORIES --branch=main \
   --service-account=projects/$PROJECT_ID/serviceAccounts/cosign@$PROJECT_ID.iam.gserviceaccount.com 

# now trigger
gcloud alpha builds triggers run cosign-trigger
```


### Verify

We are now ready to verify the images locally and using `cosign`


#### KMS

For kms keys, verify by either downloading kms public key

```bash
cd ../
gcloud kms keys versions get-public-key 1  \
  --key=key1 --keyring=cosignkr \
  --location=global --output-file=kms_pub.pem


# verify using the local key 
cosign verify --key kms_pub.pem   \
   us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753  | jq '.'

# or by api
cosign verify --key gcpkms://projects/$PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 \
      us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753 | jq '.'
```

Note this gives 

```text
Verification for us-central1-docker.pkg.dev/mineral-minutia-820/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753 --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - The signatures were verified against the specified public key
[
  {
    "critical": {
      "identity": {
        "docker-reference": "us-central1-docker.pkg.dev/mineral-minutia-820/repo1/securebuild"
      },
      "image": {
        "docker-manifest-digest": "sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753"
      },
      "type": "cosign container image signature"
    },
    "optional": {
      "key1": "value1"
    }
  }
]
```

### Transparency Log (rekor)


The OIDC flow also creates entries in the  transparency logs

TO verify,

```bash
COSIGN_EXPERIMENTAL=1  cosign verify  us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753 | jq '.'
```

gives

```text
Verification for us-central1-docker.pkg.dev/mineral-minutia-820/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753 --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - Existence of the claims in the transparency log was verified offline
  - Any certificates were verified against the Fulcio roots.
[
  {
    "critical": {
      "identity": {
        "docker-reference": "us-central1-docker.pkg.dev/mineral-minutia-820/repo1/securebuild"
      },
      "image": {
        "docker-manifest-digest": "sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753"
      },
      "type": "cosign container image signature"
    },
    "optional": {
      "1.3.6.1.4.1.57264.1.1": "https://accounts.google.com",
      "Bundle": {
        "SignedEntryTimestamp": "MEUCIQDdOUEUMeJD/yz1AlhQZr01qOb52YuzcN7JDzckeiWTvgIgHvDKcAoLXi6efRacsWOw7U6MO14Z7XrFBEm8AcJdqn4=",
        "Payload": {
          "body": "eyJhcGlWZXJzaW9uIjoiMC4wLjEiLCJraW5kIjoiaGFzaGVkcmVrb3JkIiwic3BlYyI6eyJkYXRhIjp7Imhhc2giOnsiYWxnb3JpdGhtIjoic2hhMjU2IiwidmFsdWUiOiJiMzA2NTAyMGUzMWM3MGI1ODMyZWNjNDYwYmM1YmNjYTcxOGNjZTg5NzRkMmNlNWY3Y2VhMzhhZjY4Yzg5MDkxIn19LCJzaWduYXR1cmUiOnsiY29udGVudCI6Ik1FWUNJUURseHlDZkVCclVVS0gxV252RG5PckVDdmZEQitqNHhVRThNa3EybzZZdWFnSWhBS1RSQ0tQeGo1Y1VLTGNJUmxDMUJDdENIWDUvWkFUbjNocE1XdHNBTEdEZiIsInB1YmxpY0tleSI6eyJjb250ZW50IjoiTFMwdExTMUNSVWRKVGlCRFJWSlVTVVpKUTBGVVJTMHRMUzB0Q2sxSlNVTTJWRU5EUVc1RFowRjNTVUpCWjBsVlVVMUJOamRWVGpGTWNsWlNjMUp0T0VkVmVFRXJXV3BDZGxWUmQwTm5XVWxMYjFwSmVtb3dSVUYzVFhjS1RucEZWazFDVFVkQk1WVkZRMmhOVFdNeWJHNWpNMUoyWTIxVmRWcEhWakpOVWpSM1NFRlpSRlpSVVVSRmVGWjZZVmRrZW1SSE9YbGFVekZ3WW01U2JBcGpiVEZzV2tkc2FHUkhWWGRJYUdOT1RXcE5kMDVFUVRWTmFrMTVUa1JKZVZkb1kwNU5hazEzVGtSQk5VMXFUWHBPUkVsNVYycEJRVTFHYTNkRmQxbElDa3R2V2tsNmFqQkRRVkZaU1V0dldrbDZhakJFUVZGalJGRm5RVVZJTTB0d2VqVndiWGhaUkZWdlEyYzFja1JxVDJGRk1EaEllWFJvZFcwM1YyOTZTRUlLYUZJNVZUQXpMM2RzVUU0MmNIQkllakJpTVVST1pFOXFkeTluY21aMGJuazVVSFpvYzBOUmRGZE5hVll4TjFWNGVHRlBRMEZaT0hkblowZE1UVUUwUndwQk1WVmtSSGRGUWk5M1VVVkJkMGxJWjBSQlZFSm5UbFpJVTFWRlJFUkJTMEpuWjNKQ1owVkdRbEZqUkVGNlFXUkNaMDVXU0ZFMFJVWm5VVlU1VkZkekNqZHBZbkpKYm1RM1dFUnBSbTl5VEV4dlJHRkJaR2xGZDBoM1dVUldVakJxUWtKbmQwWnZRVlV6T1ZCd2VqRlphMFZhWWpWeFRtcHdTMFpYYVhocE5Ga0tXa1E0ZDFGQldVUldVakJTUVZGSUwwSkVXWGRPU1VWNVdUSTVlbUZYWkhWUlJ6RndZbTFXZVZsWGQzUmlWMngxWkZoU2NGbFRNRFJOYWtGMVlWZEdkQXBNYldSNldsaEtNbUZYVG14WlYwNXFZak5XZFdSRE5XcGlNakIzUzFGWlMwdDNXVUpDUVVkRWRucEJRa0ZSVVdKaFNGSXdZMGhOTmt4NU9XaFpNazUyQ21SWE5UQmplVFZ1WWpJNWJtSkhWWFZaTWpsMFRVTnpSME5wYzBkQlVWRkNaemM0ZDBGUlowVklVWGRpWVVoU01HTklUVFpNZVRsb1dUSk9kbVJYTlRBS1kzazFibUl5T1c1aVIxVjFXVEk1ZEUxSlIwcENaMjl5UW1kRlJVRmtXalZCWjFGRFFraHpSV1ZSUWpOQlNGVkJNMVF3ZDJGellraEZWRXBxUjFJMFl3cHRWMk16UVhGS1MxaHlhbVZRU3pNdmFEUndlV2RET0hBM2J6UkJRVUZIU0dGR1Z5OHJVVUZCUWtGTlFWSnFRa1ZCYVVKS1FraDRhelZXU2psNk9XVk9DblJsUm5ocVpXVjZkaXR0VVZWdGVHbHpWMHhvVUd0RlZHUmhOR0ZHVVVsblZXOU1ialJUTUdKV1oxVkVUa0p0VTAxTWJVeDZaa2MxTkRGelRYWkROM01LUTFOdEx6UldVblJZZVVWM1EyZFpTVXR2V2tsNmFqQkZRWGROUkZwM1FYZGFRVWwzVFZaWFEyOVhUMlI1WkVZNGIydFVUV0ZVWkVWaUwzbGFRV3BxY0FwME5EUTJUa2h2VUhObmRDczRUMHREZUVOaVJtUXdhVGd6Ym5NeFVUWjBXblZQT0ZaQmFrRnFha2hDWWpSSVVsQXZPWFU0WnpkT2MydHNZaloyUTNkdENtaHFaWE5aUVdkMGQwdzNTVVpTV0V4dkwxSkVWbUV5UjBaR1V6bENibEZWU0ZkdmJrVXpNRDBLTFMwdExTMUZUa1FnUTBWU1ZFbEdTVU5CVkVVdExTMHRMUW89In19fX0=",
          "integratedTime": 1681082664,
          "logIndex": 17544187,
          "logID": "c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d"
        }
      },
      "Issuer": "https://accounts.google.com",
      "Subject": "cosign@mineral-minutia-820.iam.gserviceaccount.com",
      "key1": "value1"
    }
  }
]
```

Note that this is what is in the transparency log itself (`logID`, `logIndex`, etc)


decoding the `payload` using [jwt.io](jwt.io) gives json

```json
{
  "apiVersion": "0.0.1",
  "kind": "hashedrekord",
  "spec": {
    "data": {
      "hash": {
        "algorithm": "sha256",
        "value": "02277181cf8f82a917f1f7348d2838ef91261739a1f2cd315d35b4da5a7655ce"
      }
    },
    "signature": {
      "content": "MEYCIQDYXlP/p84Gt2N4jczhUx52+BW+G1EUPLuaguLQ8C3GYAIhAO+nSII+gN6KGVEI0Yromod1Vl2C2wRZNhItNKjQPwC9",
      "publicKey": {
        "content": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUM2RENDQW0rZ0F3SUJBZ0lVWC9PZ0pPWEM2b2lKQUw3SFdqRkR6cWJSNnRZd0NnWUlLb1pJemowRUF3TXcKTnpFVk1CTUdBMVVFQ2hNTWMybG5jM1J2Y21VdVpHVjJNUjR3SEFZRFZRUURFeFZ6YVdkemRHOXlaUzFwYm5SbApjbTFsWkdsaGRHVXdIaGNOTWpNd05EQTRNakl6TWpJNVdoY05Nak13TkRBNE1qSTBNakk1V2pBQU1Ga3dFd1lICktvWkl6ajBDQVFZSUtvWkl6ajBEQVFjRFFnQUVMVVl0VGdnQ1N2M3JGK2RyWTUvNkpqZmoxUXJxYlFXN1gxZ1EKSTFSS0cvOWhsVmU2NGdlSkxNQnhqTU9xN3pkdkVvb2ViRFRyNXJTNTBZQzRrbnlES0tPQ0FZNHdnZ0dLTUE0RwpBMVVkRHdFQi93UUVBd0lIZ0RBVEJnTlZIU1VFRERBS0JnZ3JCZ0VGQlFjREF6QWRCZ05WSFE0RUZnUVUxa0pvClVHY0pQdk5mempDUVVtNGVwVkZTTGlFd0h3WURWUjBqQkJnd0ZvQVUzOVBwejFZa0VaYjVxTmpwS0ZXaXhpNFkKWkQ4d1B3WURWUjBSQVFIL0JEVXdNNEV4WTI5emFXZHVRR052YzJsbmJpMTBaWE4wTFRNNE16RXlNaTVwWVcwdQpaM05sY25acFkyVmhZMk52ZFc1MExtTnZiVEFwQmdvckJnRUVBWU8vTUFFQkJCdG9kSFJ3Y3pvdkwyRmpZMjkxCmJuUnpMbWR2YjJkc1pTNWpiMjB3S3dZS0t3WUJCQUdEdnpBQkNBUWREQnRvZEhSd2N6b3ZMMkZqWTI5MWJuUnoKTG1kdmIyZHNaUzVqYjIwd2dZa0dDaXNHQVFRQjFua0NCQUlFZXdSNUFIY0FkUURkUFRCcXhzY1JNbU1aSGh5WgpaemNDb2twZXVONDhyZitIaW5LQUx5bnVqZ0FBQVlkaS8rUTlBQUFFQXdCR01FUUNJQ1JUcjFId25mbFUrbi9WCmJUc0xmajBpMmhXa2VKZkYrc1FwWW9DbWNMVTNBaUJGTnQ5ak44emtjQ0NuTVhLTjRNR1ZkS0E0eVl4QUNYb3MKMjEwSmh1cU1KekFLQmdncWhrak9QUVFEQXdObkFEQmtBakI2WmNMRU9rbXpXeGd2NVhJUW5aWnNWRk1EcXdlMApOaXk0MkM1cjJ1QWdvcEFtMVEvQk5wTnJqdFFDWmFmd1VMY0NNRStucTByd3dIa3VsREN3SkFDV1pYVUk4TzUvClltNkFjcWwwbWVFaDQwOWVRK1ZJWnpyZDdNckhhcmd0WklaMDV3PT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
      }
    }
  }
}
```

from there the base64encoded `publicKey` is what was issued during the [signing ceremony](https://docs.sigstore.dev/fulcio/certificate-issuing-overview). 

```
-----BEGIN CERTIFICATE-----
MIIC6DCCAm+gAwIBAgIUX/OgJOXC6oiJAL7HWjFDzqbR6tYwCgYIKoZIzj0EAwMw
NzEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MR4wHAYDVQQDExVzaWdzdG9yZS1pbnRl
cm1lZGlhdGUwHhcNMjMwNDA4MjIzMjI5WhcNMjMwNDA4MjI0MjI5WjAAMFkwEwYH
KoZIzj0CAQYIKoZIzj0DAQcDQgAELUYtTggCSv3rF+drY5/6Jjfj1QrqbQW7X1gQ
I1RKG/9hlVe64geJLMBxjMOq7zdvEooebDTr5rS50YC4knyDKKOCAY4wggGKMA4G
A1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAdBgNVHQ4EFgQU1kJo
UGcJPvNfzjCQUm4epVFSLiEwHwYDVR0jBBgwFoAU39Ppz1YkEZb5qNjpKFWixi4Y
ZD8wPwYDVR0RAQH/BDUwM4ExY29zaWduQGNvc2lnbi10ZXN0LTM4MzEyMi5pYW0u
Z3NlcnZpY2VhY2NvdW50LmNvbTApBgorBgEEAYO/MAEBBBtodHRwczovL2FjY291
bnRzLmdvb2dsZS5jb20wKwYKKwYBBAGDvzABCAQdDBtodHRwczovL2FjY291bnRz
Lmdvb2dsZS5jb20wgYkGCisGAQQB1nkCBAIEewR5AHcAdQDdPTBqxscRMmMZHhyZ
ZzcCokpeuN48rf+HinKALynujgAAAYdi/+Q9AAAEAwBGMEQCICRTr1HwnflU+n/V
bTsLfj0i2hWkeJfF+sQpYoCmcLU3AiBFNt9jN8zkcCCnMXKN4MGVdKA4yYxACXos
210JhuqMJzAKBggqhkjOPQQDAwNnADBkAjB6ZcLEOkmzWxgv5XIQnZZsVFMDqwe0
Niy42C5r2uAgopAm1Q/BNpNrjtQCZafwULcCME+nq0rwwHkulDCwJACWZXUI8O5/
Ym6Acql0meEh409eQ+VIZzrd7MrHargtZIZ05w==
-----END CERTIFICATE-----
```

which expanded is 

```bash
$  openssl x509 -in cosign.crt -noout -text

Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number:
            5f:f3:a0:24:e5:c2:ea:88:89:00:be:c7:5a:31:43:ce:a6:d1:ea:d6
        Signature Algorithm: ecdsa-with-SHA384
        Issuer: O = sigstore.dev, CN = sigstore-intermediate
        Validity
            Not Before: Apr  8 22:32:29 2023 GMT
            Not After : Apr  8 22:42:29 2023 GMT
        Subject: 
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
                Public-Key: (256 bit)
                pub:
                    04:2d:46:2d:4e:08:02:4a:fd:eb:17:e7:6b:63:9f:
                    fa:26:37:e3:d5:0a:ea:6d:05:bb:5f:58:10:23:54:
                    4a:1b:ff:61:95:57:ba:e2:07:89:2c:c0:71:8c:c3:
                    aa:ef:37:6f:12:8a:1e:6c:34:eb:e6:b4:b9:d1:80:
                    b8:92:7c:83:28
                ASN1 OID: prime256v1
                NIST CURVE: P-256
        X509v3 extensions:
            X509v3 Key Usage: critical
                Digital Signature
            X509v3 Extended Key Usage: 
                Code Signing
            X509v3 Subject Key Identifier: 
                D6:42:68:50:67:09:3E:F3:5F:CE:30:90:52:6E:1E:A5:51:52:2E:21
            X509v3 Authority Key Identifier: 
                DF:D3:E9:CF:56:24:11:96:F9:A8:D8:E9:28:55:A2:C6:2E:18:64:3F
            X509v3 Subject Alternative Name: critical
                email:cosign@cosign-test-383122.iam.gserviceaccount.com              <<<<<<<<<<<<<<<<<<
            1.3.6.1.4.1.57264.1.1: 
                https://accounts.google.com
            1.3.6.1.4.1.57264.1.8: 
                ..https://accounts.google.com
            CT Precertificate SCTs: 
                Signed Certificate Timestamp:
                    Version   : v1 (0x0)
                    Log ID    : DD:3D:30:6A:C6:C7:11:32:63:19:1E:1C:99:67:37:02:
                                A2:4A:5E:B8:DE:3C:AD:FF:87:8A:72:80:2F:29:EE:8E
                    Timestamp : Apr  8 22:32:30.013 2023 GMT
                    Extensions: none
                    Signature : ecdsa-with-SHA256
                                30:44:02:20:24:53:AF:51:F0:9D:F9:54:FA:7F:D5:6D:
                                3B:0B:7E:3D:22:DA:15:A4:78:97:C5:FA:C4:29:62:80:
                                A6:70:B5:37:02:20:45:36:DF:63:37:CC:E4:70:20:A7:
                                31:72:8D:E0:C1:95:74:A0:38:C9:8C:40:09:7A:2C:DB:
                                5D:09:86:EA:8C:27
    Signature Algorithm: ecdsa-with-SHA384
    Signature Value:
        30:64:02:30:7a:65:c2:c4:3a:49:b3:5b:18:2f:e5:72:10:9d:
        96:6c:54:53:03:ab:07:b4:36:2c:b8:d8:2e:6b:da:e0:20:a2:
        90:26:d5:0f:c1:36:93:6b:8e:d4:02:65:a7:f0:50:b7:02:30:
        4f:a7:ab:4a:f0:c0:79:2e:94:30:b0:24:00:96:65:75:08:f0:
        ee:7f:62:6e:80:72:a9:74:99:e1:21:e3:4f:5e:43:e5:48:67:
        3a:dd:ec:ca:c7:6a:b8:2d:64:86:74:e7
```

NOTE the OID `1.3.6.1.4.1.57264.1.1` is registered to [here](https://github.com/sigstore/fulcio/blob/main/docs/oid-info.md#directory) and denotes the OIDC Token's issuer

Now use `rekor-cli` to search for what we added to the transparency log using


* `sha` value from `hashedrekord`

```bash
$ rekor-cli search --rekor_server https://rekor.sigstore.dev \
   --sha  02277181cf8f82a917f1f7348d2838ef91261739a1f2cd315d35b4da5a7655ce

  Found matching entries (listed by UUID):
  24296fb24b8ad77a613ebee274f1633e776c09e4f9139db7d2342079e0ba59257ce0472ca3ba33ff
```

* the email for the build service account's `OIDC`

```bash
$ rekor-cli search --rekor_server https://rekor.sigstore.dev  --email cosign@$PROJECT_ID.iam.gserviceaccount.com

Found matching entries (listed by UUID):
24296fb24b8ad77a31b10e4b97faecd20e3fab5192a93a388f183f5109280267cdaa60537878f0eb
24296fb24b8ad77a613ebee274f1633e776c09e4f9139db7d2342079e0ba59257ce0472ca3ba33ff
```

note each `UUID` asserts something different:  the `signature` and another one for the `attestation`


For the `Signature`

```
rekor-cli get --rekor_server https://rekor.sigstore.dev  \
  --uuid 24296fb24b8ad77a613ebee274f1633e776c09e4f9139db7d2342079e0ba59257ce0472ca3ba33ff
```

outputs (note the `Index` value matches what we have in the "sign_oidc" build step)

```text
LogID: c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d
Index: 17475493
IntegratedTime: 2023-04-08T22:32:31Z
UUID: 24296fb24b8ad77a613ebee274f1633e776c09e4f9139db7d2342079e0ba59257ce0472ca3ba33ff
Body: {
  "HashedRekordObj": {
    "data": {
      "hash": {
        "algorithm": "sha256",
        "value": "02277181cf8f82a917f1f7348d2838ef91261739a1f2cd315d35b4da5a7655ce"
      }
    },
    "signature": {
      "content": "MEYCIQDYXlP/p84Gt2N4jczhUx52+BW+G1EUPLuaguLQ8C3GYAIhAO+nSII+gN6KGVEI0Yromod1Vl2C2wRZNhItNKjQPwC9",
      "publicKey": {
        "content": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUM2RENDQW0rZ0F3SUJBZ0lVWC9PZ0pPWEM2b2lKQUw3SFdqRkR6cWJSNnRZd0NnWUlLb1pJemowRUF3TXcKTnpFVk1CTUdBMVVFQ2hNTWMybG5jM1J2Y21VdVpHVjJNUjR3SEFZRFZRUURFeFZ6YVdkemRHOXlaUzFwYm5SbApjbTFsWkdsaGRHVXdIaGNOTWpNd05EQTRNakl6TWpJNVdoY05Nak13TkRBNE1qSTBNakk1V2pBQU1Ga3dFd1lICktvWkl6ajBDQVFZSUtvWkl6ajBEQVFjRFFnQUVMVVl0VGdnQ1N2M3JGK2RyWTUvNkpqZmoxUXJxYlFXN1gxZ1EKSTFSS0cvOWhsVmU2NGdlSkxNQnhqTU9xN3pkdkVvb2ViRFRyNXJTNTBZQzRrbnlES0tPQ0FZNHdnZ0dLTUE0RwpBMVVkRHdFQi93UUVBd0lIZ0RBVEJnTlZIU1VFRERBS0JnZ3JCZ0VGQlFjREF6QWRCZ05WSFE0RUZnUVUxa0pvClVHY0pQdk5mempDUVVtNGVwVkZTTGlFd0h3WURWUjBqQkJnd0ZvQVUzOVBwejFZa0VaYjVxTmpwS0ZXaXhpNFkKWkQ4d1B3WURWUjBSQVFIL0JEVXdNNEV4WTI5emFXZHVRR052YzJsbmJpMTBaWE4wTFRNNE16RXlNaTVwWVcwdQpaM05sY25acFkyVmhZMk52ZFc1MExtTnZiVEFwQmdvckJnRUVBWU8vTUFFQkJCdG9kSFJ3Y3pvdkwyRmpZMjkxCmJuUnpMbWR2YjJkc1pTNWpiMjB3S3dZS0t3WUJCQUdEdnpBQkNBUWREQnRvZEhSd2N6b3ZMMkZqWTI5MWJuUnoKTG1kdmIyZHNaUzVqYjIwd2dZa0dDaXNHQVFRQjFua0NCQUlFZXdSNUFIY0FkUURkUFRCcXhzY1JNbU1aSGh5WgpaemNDb2twZXVONDhyZitIaW5LQUx5bnVqZ0FBQVlkaS8rUTlBQUFFQXdCR01FUUNJQ1JUcjFId25mbFUrbi9WCmJUc0xmajBpMmhXa2VKZkYrc1FwWW9DbWNMVTNBaUJGTnQ5ak44emtjQ0NuTVhLTjRNR1ZkS0E0eVl4QUNYb3MKMjEwSmh1cU1KekFLQmdncWhrak9QUVFEQXdObkFEQmtBakI2WmNMRU9rbXpXeGd2NVhJUW5aWnNWRk1EcXdlMApOaXk0MkM1cjJ1QWdvcEFtMVEvQk5wTnJqdFFDWmFmd1VMY0NNRStucTByd3dIa3VsREN3SkFDV1pYVUk4TzUvClltNkFjcWwwbWVFaDQwOWVRK1ZJWnpyZDdNckhhcmd0WklaMDV3PT0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
      }
    }
  }
}
```

the certificate from the decoded publicKey outputs the cert issued by falcio

```
-----BEGIN CERTIFICATE-----
MIIC6DCCAm+gAwIBAgIUX/OgJOXC6oiJAL7HWjFDzqbR6tYwCgYIKoZIzj0EAwMw
NzEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MR4wHAYDVQQDExVzaWdzdG9yZS1pbnRl
cm1lZGlhdGUwHhcNMjMwNDA4MjIzMjI5WhcNMjMwNDA4MjI0MjI5WjAAMFkwEwYH
KoZIzj0CAQYIKoZIzj0DAQcDQgAELUYtTggCSv3rF+drY5/6Jjfj1QrqbQW7X1gQ
I1RKG/9hlVe64geJLMBxjMOq7zdvEooebDTr5rS50YC4knyDKKOCAY4wggGKMA4G
A1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAdBgNVHQ4EFgQU1kJo
UGcJPvNfzjCQUm4epVFSLiEwHwYDVR0jBBgwFoAU39Ppz1YkEZb5qNjpKFWixi4Y
ZD8wPwYDVR0RAQH/BDUwM4ExY29zaWduQGNvc2lnbi10ZXN0LTM4MzEyMi5pYW0u
Z3NlcnZpY2VhY2NvdW50LmNvbTApBgorBgEEAYO/MAEBBBtodHRwczovL2FjY291
bnRzLmdvb2dsZS5jb20wKwYKKwYBBAGDvzABCAQdDBtodHRwczovL2FjY291bnRz
Lmdvb2dsZS5jb20wgYkGCisGAQQB1nkCBAIEewR5AHcAdQDdPTBqxscRMmMZHhyZ
ZzcCokpeuN48rf+HinKALynujgAAAYdi/+Q9AAAEAwBGMEQCICRTr1HwnflU+n/V
bTsLfj0i2hWkeJfF+sQpYoCmcLU3AiBFNt9jN8zkcCCnMXKN4MGVdKA4yYxACXos
210JhuqMJzAKBggqhkjOPQQDAwNnADBkAjB6ZcLEOkmzWxgv5XIQnZZsVFMDqwe0
Niy42C5r2uAgopAm1Q/BNpNrjtQCZafwULcCME+nq0rwwHkulDCwJACWZXUI8O5/
Ym6Acql0meEh409eQ+VIZzrd7MrHargtZIZ05w==
-----END CERTIFICATE-----




Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number:
            5f:f3:a0:24:e5:c2:ea:88:89:00:be:c7:5a:31:43:ce:a6:d1:ea:d6
        Signature Algorithm: ecdsa-with-SHA384
        Issuer: O = sigstore.dev, CN = sigstore-intermediate
        Validity
            Not Before: Apr  8 22:32:29 2023 GMT
            Not After : Apr  8 22:42:29 2023 GMT
        Subject: 
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
                Public-Key: (256 bit)
                pub:
                    04:2d:46:2d:4e:08:02:4a:fd:eb:17:e7:6b:63:9f:
                    fa:26:37:e3:d5:0a:ea:6d:05:bb:5f:58:10:23:54:
                    4a:1b:ff:61:95:57:ba:e2:07:89:2c:c0:71:8c:c3:
                    aa:ef:37:6f:12:8a:1e:6c:34:eb:e6:b4:b9:d1:80:
                    b8:92:7c:83:28
                ASN1 OID: prime256v1
                NIST CURVE: P-256
        X509v3 extensions:
            X509v3 Key Usage: critical
                Digital Signature
            X509v3 Extended Key Usage: 
                Code Signing
            X509v3 Subject Key Identifier: 
                D6:42:68:50:67:09:3E:F3:5F:CE:30:90:52:6E:1E:A5:51:52:2E:21
            X509v3 Authority Key Identifier: 
                DF:D3:E9:CF:56:24:11:96:F9:A8:D8:E9:28:55:A2:C6:2E:18:64:3F
            X509v3 Subject Alternative Name: critical
                email:cosign@cosign-test-383122.iam.gserviceaccount.com            <<<<<<<<<
            1.3.6.1.4.1.57264.1.1: 
                https://accounts.google.com
            1.3.6.1.4.1.57264.1.8: 
                ..https://accounts.google.com
            CT Precertificate SCTs: 
                Signed Certificate Timestamp:
                    Version   : v1 (0x0)
                    Log ID    : DD:3D:30:6A:C6:C7:11:32:63:19:1E:1C:99:67:37:02:
                                A2:4A:5E:B8:DE:3C:AD:FF:87:8A:72:80:2F:29:EE:8E
                    Timestamp : Apr  8 22:32:30.013 2023 GMT
                    Extensions: none
                    Signature : ecdsa-with-SHA256
                                30:44:02:20:24:53:AF:51:F0:9D:F9:54:FA:7F:D5:6D:
                                3B:0B:7E:3D:22:DA:15:A4:78:97:C5:FA:C4:29:62:80:
                                A6:70:B5:37:02:20:45:36:DF:63:37:CC:E4:70:20:A7:
                                31:72:8D:E0:C1:95:74:A0:38:C9:8C:40:09:7A:2C:DB:
                                5D:09:86:EA:8C:27
    Signature Algorithm: ecdsa-with-SHA384
    Signature Value:
        30:64:02:30:7a:65:c2:c4:3a:49:b3:5b:18:2f:e5:72:10:9d:
        96:6c:54:53:03:ab:07:b4:36:2c:b8:d8:2e:6b:da:e0:20:a2:
        90:26:d5:0f:c1:36:93:6b:8e:d4:02:65:a7:f0:50:b7:02:30:
        4f:a7:ab:4a:f0:c0:79:2e:94:30:b0:24:00:96:65:75:08:f0:
        ee:7f:62:6e:80:72:a9:74:99:e1:21:e3:4f:5e:43:e5:48:67:
        3a:dd:ec:ca:c7:6a:b8:2d:64:86:74:e7
```


for the `Attestation`

```bash
rekor-cli get --rekor_server https://rekor.sigstore.dev \
   --uuid 24296fb24b8ad77a31b10e4b97faecd20e3fab5192a93a388f183f5109280267cdaa60537878f0eb
```

gives (again, note the attestations and `Index` that matches the "attest_oidc" step)

```text
LogID: c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d
Attestation: {"_type":"https://in-toto.io/Statement/v0.1","predicateType":"cosign.sigstore.dev/attestation/v1","subject":[{"name":"us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild","digest":{"sha256":"83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753"}}],"predicate":{"Data":"{ \"projectid\": \"cosign-test-383122\", \"buildid\": \"0a774360-d57e-41af-af8b-76dd4109c47c\", \"foo\":\"bar\", \"commitsha\": \"52dc8c1979d7e6b56a5a253dbd79028842752b08\" }","Timestamp":"2023-04-08T22:32:34Z"}}
Index: 17475497
IntegratedTime: 2023-04-08T22:32:35Z
UUID: 24296fb24b8ad77a31b10e4b97faecd20e3fab5192a93a388f183f5109280267cdaa60537878f0eb
Body: {
  "IntotoObj": {
    "content": {
      "hash": {
        "algorithm": "sha256",
        "value": "b13908ecec6666c94a7ed985f51dab3dd42f24fa64650f247db270f9d89f2d79"
      },
      "payloadHash": {
        "algorithm": "sha256",
        "value": "18b33fdbf413ab8b5d08a1926754787ccffdc0a65695b01b11247900604d039a"
      }
    },
    "publicKey": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUM2VENDQW5DZ0F3SUJBZ0lVS0ZleXk4L2NHYjBmUHN5M1VPWDRnSzFkMVlnd0NnWUlLb1pJemowRUF3TXcKTnpFVk1CTUdBMVVFQ2hNTWMybG5jM1J2Y21VdVpHVjJNUjR3SEFZRFZRUURFeFZ6YVdkemRHOXlaUzFwYm5SbApjbTFsWkdsaGRHVXdIaGNOTWpNd05EQTRNakl6TWpNMFdoY05Nak13TkRBNE1qSTBNak0wV2pBQU1Ga3dFd1lICktvWkl6ajBDQVFZSUtvWkl6ajBEQVFjRFFnQUVld3JQVGVmaDBWMDExbWs3d2ZEdjViQ2xRMTNpckF1MWNqUDMKZm9HV0tXcVV5NkN6TjI1Tk1vRTczSFpMaWJIK0Y1S0tuV2xFYVhab1N3ZHlCWU10OWFPQ0FZOHdnZ0dMTUE0RwpBMVVkRHdFQi93UUVBd0lIZ0RBVEJnTlZIU1VFRERBS0JnZ3JCZ0VGQlFjREF6QWRCZ05WSFE0RUZnUVUyR2ljCjNUMVgxQXZuaVlUVW9TZHJmOVJOMGJBd0h3WURWUjBqQkJnd0ZvQVUzOVBwejFZa0VaYjVxTmpwS0ZXaXhpNFkKWkQ4d1B3WURWUjBSQVFIL0JEVXdNNEV4WTI5emFXZHVRR052YzJsbmJpMTBaWE4wTFRNNE16RXlNaTVwWVcwdQpaM05sY25acFkyVmhZMk52ZFc1MExtTnZiVEFwQmdvckJnRUVBWU8vTUFFQkJCdG9kSFJ3Y3pvdkwyRmpZMjkxCmJuUnpMbWR2YjJkc1pTNWpiMjB3S3dZS0t3WUJCQUdEdnpBQkNBUWREQnRvZEhSd2N6b3ZMMkZqWTI5MWJuUnoKTG1kdmIyZHNaUzVqYjIwd2dZb0dDaXNHQVFRQjFua0NCQUlFZkFSNkFIZ0FkZ0RkUFRCcXhzY1JNbU1aSGh5WgpaemNDb2twZXVONDhyZitIaW5LQUx5bnVqZ0FBQVlkaS8vVkFBQUFFQXdCSE1FVUNJRkdKRUZiWmlrQ0RPcWgwClc4eHJpVzBVL1o1Vm9MditmSWRYeTJpMHRaZkJBaUVBM2NRMnlCRzIzVDIzVzFaRSswM2JTR254UmFTcVdGbCsKUEdBVnlES1FweEV3Q2dZSUtvWkl6ajBFQXdNRFp3QXdaQUl3YTJDZWZwMGZTWEo3Tk14N0xSS0R5YUZZRElmVwo1amRUaXNrNmZFckJLS0Q2c2l1TlB3Q0o1SWZKeVNGTm11b1pBakF6QldPc2FRbGp2V0NyK0l4S0xGME9UZEttCmhBSXdLcVFCREFzK2Z3VDkzbHZVQVczM2hTUCs0dE55MC85ZUJKST0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo="
  }
}
```

the decoded public key gives

```bash
-----BEGIN CERTIFICATE-----
MIICvTCCAkOgAwIBAgIUPFs1vvKZpTTL2QoBRMMMoBenoswwCgYIKoZIzj0EAwMw
NzEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MR4wHAYDVQQDExVzaWdzdG9yZS1pbnRl
cm1lZGlhdGUwHhcNMjIxMDExMDk1MjI5WhcNMjIxMDExMTAwMjI5WjAAMFkwEwYH
KoZIzj0CAQYIKoZIzj0DAQcDQgAEUrjCJ1a21A5e+8T+P1Sb8tB4kz6t/XWQfpaT
+LuEvmIqKseMPiNCXlQ0cuJDMBCPVOvqzGGPXl2zwQSBZFtOsKOCAWIwggFeMA4G
A1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAdBgNVHQ4EFgQUmBj5
scvtxYDcrdXOCzoCdyptj50wHwYDVR0jBBgwFoAU39Ppz1YkEZb5qNjpKFWixi4Y
ZD8wPwYDVR0RAQH/BDUwM4ExY29zaWduQGNvc2lnbi10ZXN0LTM2NTIwOS5pYW0u
Z3NlcnZpY2VhY2NvdW50LmNvbTApBgorBgEEAYO/MAEBBBtodHRwczovL2FjY291
bnRzLmdvb2dsZS5jb20wgYoGCisGAQQB1nkCBAIEfAR6AHgAdgAIYJLwKFL/aEXR
0WsnhJxFZxisFj3DONJt5rwiBjZvcgAAAYPGdcIHAAAEAwBHMEUCIERwE0LQXDJk
7Geiq4D9vpnscmhL5I/icEun+kP5/pxKAiEAuxJSN4zPlYqd7hbdItcuELj3b/jc
718pD/y6oo5y7lUwCgYIKoZIzj0EAwMDaAAwZQIxAMt0QpEeQrjooEx7LKLUtSja
MCCzqc+Qkd1i2DxeT6Nob6oqo9izwOUEpODgkPrd0gIwbnyLmdDetq7jSrK94ep4
0no7Bcns7404NXcvXZMGQi64pmTHUmc0uQIxr00VSGVf
-----END CERTIFICATE-----

Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number:
            28:57:b2:cb:cf:dc:19:bd:1f:3e:cc:b7:50:e5:f8:80:ad:5d:d5:88
        Signature Algorithm: ecdsa-with-SHA384
        Issuer: O = sigstore.dev, CN = sigstore-intermediate
        Validity
            Not Before: Apr  8 22:32:34 2023 GMT
            Not After : Apr  8 22:42:34 2023 GMT
        Subject: 
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
                Public-Key: (256 bit)
                pub:
                    04:7b:0a:cf:4d:e7:e1:d1:5d:35:d6:69:3b:c1:f0:
                    ef:e5:b0:a5:43:5d:e2:ac:0b:b5:72:33:f7:7e:81:
                    96:29:6a:94:cb:a0:b3:37:6e:4d:32:81:3b:dc:76:
                    4b:89:b1:fe:17:92:8a:9d:69:44:69:76:68:4b:07:
                    72:05:83:2d:f5
                ASN1 OID: prime256v1
                NIST CURVE: P-256
        X509v3 extensions:
            X509v3 Key Usage: critical
                Digital Signature
            X509v3 Extended Key Usage: 
                Code Signing
            X509v3 Subject Key Identifier: 
                D8:68:9C:DD:3D:57:D4:0B:E7:89:84:D4:A1:27:6B:7F:D4:4D:D1:B0
            X509v3 Authority Key Identifier: 
                DF:D3:E9:CF:56:24:11:96:F9:A8:D8:E9:28:55:A2:C6:2E:18:64:3F
            X509v3 Subject Alternative Name: critical
                email:cosign@cosign-test-383122.iam.gserviceaccount.com             <<<<<<
            1.3.6.1.4.1.57264.1.1: 
                https://accounts.google.com
            1.3.6.1.4.1.57264.1.8: 
                ..https://accounts.google.com
            CT Precertificate SCTs: 
                Signed Certificate Timestamp:
                    Version   : v1 (0x0)
                    Log ID    : DD:3D:30:6A:C6:C7:11:32:63:19:1E:1C:99:67:37:02:
                                A2:4A:5E:B8:DE:3C:AD:FF:87:8A:72:80:2F:29:EE:8E
                    Timestamp : Apr  8 22:32:34.368 2023 GMT
                    Extensions: none
                    Signature : ecdsa-with-SHA256
                                30:45:02:20:51:89:10:56:D9:8A:40:83:3A:A8:74:5B:
                                CC:6B:89:6D:14:FD:9E:55:A0:BB:FE:7C:87:57:CB:68:
                                B4:B5:97:C1:02:21:00:DD:C4:36:C8:11:B6:DD:3D:B7:
                                5B:56:44:FB:4D:DB:48:69:F1:45:A4:AA:58:59:7E:3C:
                                60:15:C8:32:90:A7:11
    Signature Algorithm: ecdsa-with-SHA384
    Signature Value:
        30:64:02:30:6b:60:9e:7e:9d:1f:49:72:7b:34:cc:7b:2d:12:
        83:c9:a1:58:0c:87:d6:e6:37:53:8a:c9:3a:7c:4a:c1:28:a0:
        fa:b2:2b:8d:3f:00:89:e4:87:c9:c9:21:4d:9a:ea:19:02:30:
        33:05:63:ac:69:09:63:bd:60:ab:f8:8c:4a:2c:5d:0e:4d:d2:
        a6:84:02:30:2a:a4:01:0c:0b:3e:7f:04:fd:de:5b:d4:01:6d:
        f7:85:23:fe:e2:d3:72:d3:ff:5e:04:92
```


(just note the timestamps on the two certs are like 2s apart (which was about when the build steps happened for each step;  also note the email SAN is the sam))


### Crane

You can also view the registry manifest for the signature using [crane](github.com/google/go-containerregistry/cmd/crane)

You'll see the two signatures (one for KMS, another larger one for the OIDC signature metadata)

```text
# go install github.com/google/go-containerregistry/cmd/crane@latest

$ crane  manifest us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild:sha256-83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753.sig | jq '.'


{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": {
    "mediaType": "application/vnd.oci.image.config.v1+json",
    "size": 352,
    "digest": "sha256:526fa1b18d988772604ef82c1e26b48e628e1c113c538f66446f350c68a7a8a9"
  },
  "layers": [
    {
      "mediaType": "application/vnd.dev.cosign.simplesigning.v1+json",
      "size": 292,
      "digest": "sha256:02277181cf8f82a917f1f7348d2838ef91261739a1f2cd315d35b4da5a7655ce",
      "annotations": {
        "dev.cosignproject.cosign/signature": "MEYCIQDtwWrh87dfPpw+jqmnYdffSEdrEZ3+TNrxhdSeJDZq7wIhAN1pdzfBUDygKMX2ZecEnM/zHp2SOfc2kJ9kXxKCIuJs"
      }
    },
    {
      "mediaType": "application/vnd.dev.cosign.simplesigning.v1+json",
      "size": 292,
      "digest": "sha256:02277181cf8f82a917f1f7348d2838ef91261739a1f2cd315d35b4da5a7655ce",
      "annotations": {
        "dev.cosignproject.cosign/signature": "MEYCIQDYXlP/p84Gt2N4jczhUx52+BW+G1EUPLuaguLQ8C3GYAIhAO+nSII+gN6KGVEI0Yromod1Vl2C2wRZNhItNKjQPwC9",
        "dev.sigstore.cosign/bundle": "{\"SignedEntryTimestamp\":\"MEUCIGeSCe6KjjOewSm74+gj6mnyUlEte051cRtfHfKTM4C5AiEA5SSCDnViFXf2qzqcdtfnlhAKa5L9G7OyRY5khBht5Kc=\",\"Payload\":{\"body\":\"eyJhcGlWZXJzaW9uIjoiMC4wLjEiLCJraW5kIjoiaGFzaGVkcmVrb3JkIiwic3BlYyI6eyJkYXRhIjp7Imhhc2giOnsiYWxnb3JpdGhtIjoic2hhMjU2IiwidmFsdWUiOiIwMjI3NzE4MWNmOGY4MmE5MTdmMWY3MzQ4ZDI4MzhlZjkxMjYxNzM5YTFmMmNkMzE1ZDM1YjRkYTVhNzY1NWNlIn19LCJzaWduYXR1cmUiOnsiY29udGVudCI6Ik1FWUNJUURZWGxQL3A4NEd0Mk40amN6aFV4NTIrQlcrRzFFVVBMdWFndUxROEMzR1lBSWhBTytuU0lJK2dONktHVkVJMFlyb21vZDFWbDJDMndSWk5oSXROS2pRUHdDOSIsInB1YmxpY0tleSI6eyJjb250ZW50IjoiTFMwdExTMUNSVWRKVGlCRFJWSlVTVVpKUTBGVVJTMHRMUzB0Q2sxSlNVTTJSRU5EUVcwclowRjNTVUpCWjBsVldDOVBaMHBQV0VNMmIybEtRVXczU0ZkcVJrUjZjV0pTTm5SWmQwTm5XVWxMYjFwSmVtb3dSVUYzVFhjS1RucEZWazFDVFVkQk1WVkZRMmhOVFdNeWJHNWpNMUoyWTIxVmRWcEhWakpOVWpSM1NFRlpSRlpSVVVSRmVGWjZZVmRrZW1SSE9YbGFVekZ3WW01U2JBcGpiVEZzV2tkc2FHUkhWWGRJYUdOT1RXcE5kMDVFUVRSTmFrbDZUV3BKTlZkb1kwNU5hazEzVGtSQk5FMXFTVEJOYWtrMVYycEJRVTFHYTNkRmQxbElDa3R2V2tsNmFqQkRRVkZaU1V0dldrbDZhakJFUVZGalJGRm5RVVZNVlZsMFZHZG5RMU4yTTNKR0syUnlXVFV2TmtwcVptb3hVWEp4WWxGWE4xZ3haMUVLU1RGU1MwY3ZPV2hzVm1VMk5HZGxTa3hOUW5ocVRVOXhOM3BrZGtWdmIyVmlSRlJ5TlhKVE5UQlpRelJyYm5sRVMwdFBRMEZaTkhkblowZExUVUUwUndwQk1WVmtSSGRGUWk5M1VVVkJkMGxJWjBSQlZFSm5UbFpJVTFWRlJFUkJTMEpuWjNKQ1owVkdRbEZqUkVGNlFXUkNaMDVXU0ZFMFJVWm5VVlV4YTBwdkNsVkhZMHBRZGs1bWVtcERVVlZ0TkdWd1ZrWlRUR2xGZDBoM1dVUldVakJxUWtKbmQwWnZRVlV6T1ZCd2VqRlphMFZhWWpWeFRtcHdTMFpYYVhocE5Ga0tXa1E0ZDFCM1dVUldVakJTUVZGSUwwSkVWWGROTkVWNFdUSTVlbUZYWkhWUlIwNTJZekpzYm1KcE1UQmFXRTR3VEZSTk5FMTZSWGxOYVRWd1dWY3dkUXBhTTA1c1kyNWFjRmt5Vm1oWk1rNTJaRmMxTUV4dFRuWmlWRUZ3UW1kdmNrSm5SVVZCV1U4dlRVRkZRa0pDZEc5a1NGSjNZM3B2ZGt3eVJtcFpNamt4Q21KdVVucE1iV1IyWWpKa2MxcFROV3BpTWpCM1MzZFpTMHQzV1VKQ1FVZEVkbnBCUWtOQlVXUkVRblJ2WkVoU2QyTjZiM1pNTWtacVdUSTVNV0p1VW5vS1RHMWtkbUl5WkhOYVV6VnFZakl3ZDJkWmEwZERhWE5IUVZGUlFqRnVhME5DUVVsRlpYZFNOVUZJWTBGa1VVUmtVRlJDY1hoelkxSk5iVTFhU0doNVdncGFlbU5EYjJ0d1pYVk9ORGh5Wml0SWFXNUxRVXg1Ym5WcVowRkJRVmxrYVM4clVUbEJRVUZGUVhkQ1IwMUZVVU5KUTFKVWNqRklkMjVtYkZVcmJpOVdDbUpVYzB4bWFqQnBNbWhYYTJWS1prWXJjMUZ3V1c5RGJXTk1WVE5CYVVKR1RuUTVhazQ0ZW10alEwTnVUVmhMVGpSTlIxWmtTMEUwZVZsNFFVTlliM01LTWpFd1NtaDFjVTFLZWtGTFFtZG5jV2hyYWs5UVVWRkVRWGRPYmtGRVFtdEJha0kyV21OTVJVOXJiWHBYZUdkMk5WaEpVVzVhV25OV1JrMUVjWGRsTUFwT2FYazBNa00xY2pKMVFXZHZjRUZ0TVZFdlFrNXdUbkpxZEZGRFdtRm1kMVZNWTBOTlJTdHVjVEJ5ZDNkSWEzVnNSRU4zU2tGRFYxcFlWVWs0VHpVdkNsbHROa0ZqY1d3d2JXVkZhRFF3T1dWUksxWkpXbnB5WkRkTmNraGhjbWQwV2tsYU1EVjNQVDBLTFMwdExTMUZUa1FnUTBWU1ZFbEdTVU5CVkVVdExTMHRMUW89In19fX0=\",\"integratedTime\":1680993151,\"logIndex\":17475493,\"logID\":\"c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d\"}}",
        "dev.sigstore.cosign/certificate": "-----BEGIN CERTIFICATE-----\nMIIC6DCCAm+gAwIBAgIUX/OgJOXC6oiJAL7HWjFDzqbR6tYwCgYIKoZIzj0EAwMw\nNzEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MR4wHAYDVQQDExVzaWdzdG9yZS1pbnRl\ncm1lZGlhdGUwHhcNMjMwNDA4MjIzMjI5WhcNMjMwNDA4MjI0MjI5WjAAMFkwEwYH\nKoZIzj0CAQYIKoZIzj0DAQcDQgAELUYtTggCSv3rF+drY5/6Jjfj1QrqbQW7X1gQ\nI1RKG/9hlVe64geJLMBxjMOq7zdvEooebDTr5rS50YC4knyDKKOCAY4wggGKMA4G\nA1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAdBgNVHQ4EFgQU1kJo\nUGcJPvNfzjCQUm4epVFSLiEwHwYDVR0jBBgwFoAU39Ppz1YkEZb5qNjpKFWixi4Y\nZD8wPwYDVR0RAQH/BDUwM4ExY29zaWduQGNvc2lnbi10ZXN0LTM4MzEyMi5pYW0u\nZ3NlcnZpY2VhY2NvdW50LmNvbTApBgorBgEEAYO/MAEBBBtodHRwczovL2FjY291\nbnRzLmdvb2dsZS5jb20wKwYKKwYBBAGDvzABCAQdDBtodHRwczovL2FjY291bnRz\nLmdvb2dsZS5jb20wgYkGCisGAQQB1nkCBAIEewR5AHcAdQDdPTBqxscRMmMZHhyZ\nZzcCokpeuN48rf+HinKALynujgAAAYdi/+Q9AAAEAwBGMEQCICRTr1HwnflU+n/V\nbTsLfj0i2hWkeJfF+sQpYoCmcLU3AiBFNt9jN8zkcCCnMXKN4MGVdKA4yYxACXos\n210JhuqMJzAKBggqhkjOPQQDAwNnADBkAjB6ZcLEOkmzWxgv5XIQnZZsVFMDqwe0\nNiy42C5r2uAgopAm1Q/BNpNrjtQCZafwULcCME+nq0rwwHkulDCwJACWZXUI8O5/\nYm6Acql0meEh409eQ+VIZzrd7MrHargtZIZ05w==\n-----END CERTIFICATE-----\n",
        "dev.sigstore.cosign/chain": "-----BEGIN CERTIFICATE-----\nMIICGjCCAaGgAwIBAgIUALnViVfnU0brJasmRkHrn/UnfaQwCgYIKoZIzj0EAwMw\nKjEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MREwDwYDVQQDEwhzaWdzdG9yZTAeFw0y\nMjA0MTMyMDA2MTVaFw0zMTEwMDUxMzU2NThaMDcxFTATBgNVBAoTDHNpZ3N0b3Jl\nLmRldjEeMBwGA1UEAxMVc2lnc3RvcmUtaW50ZXJtZWRpYXRlMHYwEAYHKoZIzj0C\nAQYFK4EEACIDYgAE8RVS/ysH+NOvuDZyPIZtilgUF9NlarYpAd9HP1vBBH1U5CV7\n7LSS7s0ZiH4nE7Hv7ptS6LvvR/STk798LVgMzLlJ4HeIfF3tHSaexLcYpSASr1kS\n0N/RgBJz/9jWCiXno3sweTAOBgNVHQ8BAf8EBAMCAQYwEwYDVR0lBAwwCgYIKwYB\nBQUHAwMwEgYDVR0TAQH/BAgwBgEB/wIBADAdBgNVHQ4EFgQU39Ppz1YkEZb5qNjp\nKFWixi4YZD8wHwYDVR0jBBgwFoAUWMAeX5FFpWapesyQoZMi0CrFxfowCgYIKoZI\nzj0EAwMDZwAwZAIwPCsQK4DYiZYDPIaDi5HFKnfxXx6ASSVmERfsynYBiX2X6SJR\nnZU84/9DZdnFvvxmAjBOt6QpBlc4J/0DxvkTCqpclvziL6BCCPnjdlIB3Pu3BxsP\nmygUY7Ii2zbdCdliiow=\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\nMIIB9zCCAXygAwIBAgIUALZNAPFdxHPwjeDloDwyYChAO/4wCgYIKoZIzj0EAwMw\nKjEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MREwDwYDVQQDEwhzaWdzdG9yZTAeFw0y\nMTEwMDcxMzU2NTlaFw0zMTEwMDUxMzU2NThaMCoxFTATBgNVBAoTDHNpZ3N0b3Jl\nLmRldjERMA8GA1UEAxMIc2lnc3RvcmUwdjAQBgcqhkjOPQIBBgUrgQQAIgNiAAT7\nXeFT4rb3PQGwS4IajtLk3/OlnpgangaBclYpsYBr5i+4ynB07ceb3LP0OIOZdxex\nX69c5iVuyJRQ+Hz05yi+UF3uBWAlHpiS5sh0+H2GHE7SXrk1EC5m1Tr19L9gg92j\nYzBhMA4GA1UdDwEB/wQEAwIBBjAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBRY\nwB5fkUWlZql6zJChkyLQKsXF+jAfBgNVHSMEGDAWgBRYwB5fkUWlZql6zJChkyLQ\nKsXF+jAKBggqhkjOPQQDAwNpADBmAjEAj1nHeXZp+13NWBNa+EDsDP8G1WWg1tCM\nWP/WHPqpaVo0jhsweNFZgSs0eE7wYI4qAjEA2WB9ot98sIkoF3vZYdd3/VtWB5b9\nTNMea7Ix/stJ5TfcLLeABLE4BNJOsQ4vnBHJ\n-----END CERTIFICATE-----"
      }
    }
  ]
}

```

![images/registry.png](images/registry.png)

##### signature manifest:

![images/signature_manifest.png](images/signature_manifest.png)

##### attestation manifest: 

![images/attestation_manifest.png](images/attestation_manifest.png)

### Verify Attestations using rego Policy

Since we added in attestations steps, you can verify them using a rego policy:

```rego
package signature

import data.signature.verified

default allow = false

allow {
    input.predicateType == "cosign.sigstore.dev/attestation/v1"

    predicates := json.unmarshal(input.predicate.Data)
    predicates.foo == "bar"
}
```

What that policy enforces is to check if a predicate called 'foo' with a value of 'bar' is included in a json Data attribute.  You can verify anything else but thats as much rego i know. FWIW, the build step also included the `commitsha` from the source repo too

```json
{ 
    "projectid": "$PROJECT_ID", 
    "buildid": "$BUILD_ID", 
    "foo": "bar",
    "commitsha": "foo"
}
```

For the KMS based signature:

```text
#  cosign attest --key gcpkms://projects/$PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 --predicate predicate.json \
#     us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753

cosign verify-attestation \
  --key gcpkms://projects/$PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 \
    --policy policy.rego    \
      us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753  | jq '.'

will be validating against Rego policies: [policy.rego]

Verification for us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753 --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - The signatures were verified against the specified public key
{
  "payloadType": "application/vnd.in-toto+json",
  "payload": "eyJfdHlwZSI6Imh0dHBzOi8vaW4tdG90by5pby9TdGF0ZW1lbnQvdjAuMSIsInByZWRpY2F0ZVR5cGUiOiJjb3NpZ24uc2lnc3RvcmUuZGV2L2F0dGVzdGF0aW9uL3YxIiwic3ViamVjdCI6W3sibmFtZSI6InVzLWNlbnRyYWwxLWRvY2tlci5wa2cuZGV2L2Nvc2lnbi10ZXN0LTM4MzEyMi9yZXBvMS9zZWN1cmVidWlsZCIsImRpZ2VzdCI6eyJzaGEyNTYiOiI4M2FiMmJhNjY4OTcxM2YyZDY4MTA0Y2QyMDhmZWFkZmViZGQ2YmM4ODFjNDU1ZGNiNTVkMmI0NWFjM2EwNzUzIn19XSwicHJlZGljYXRlIjp7IkRhdGEiOiJ7IFwicHJvamVjdGlkXCI6IFwiY29zaWduLXRlc3QtMzgzMTIyXCIsIFwiYnVpbGRpZFwiOiBcIjBhNzc0MzYwLWQ1N2UtNDFhZi1hZjhiLTc2ZGQ0MTA5YzQ3Y1wiLCBcImZvb1wiOlwiYmFyXCIsIFwiY29tbWl0c2hhXCI6IFwiNTJkYzhjMTk3OWQ3ZTZiNTZhNWEyNTNkYmQ3OTAyODg0Mjc1MmIwOFwiIH0iLCJUaW1lc3RhbXAiOiIyMDIzLTA0LTA4VDIyOjMyOjI2WiJ9fQ==",
  "signatures": [
    {
      "keyid": "",
      "sig": "MEQCIBQsk8yCpb+MoM5Jq8Mx7a+QsFUJNmW0CLboJjvU7HkmAiBug+e7gpYL/NzoguzhZrYJ3mQ3RnhE6U2LziK5SuQlIA=="
    }
  ]
}
```

the decoded payload is

```json
{
  "_type": "https://in-toto.io/Statement/v0.1",
  "predicateType": "cosign.sigstore.dev/attestation/v1",
  "subject": [
    {
      "name": "us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild",
      "digest": {
        "sha256": "83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753"
      }
    }
  ],
  "predicate": {
    "Data": "{ 
      \"projectid\": \"cosign-test-383122\", 
      \"buildid\": \"0a774360-d57e-41af-af8b-76dd4109c47c\", 
      \"foo\":\"bar\", 
      \"commitsha\": \"52dc8c1979d7e6b56a5a253dbd79028842752b08\" 
    }",
    "Timestamp": "2023-04-08T22:32:26Z"
  }
}
```

Note the commit hash (`52dc8c1979d7e6b56a5a253dbd79028842752b08`).  you can define a rego to validate that too

for the OIDC based signature,


```bash
COSIGN_EXPERIMENTAL=1 cosign verify-attestation  --policy policy.rego    \
        us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753  | jq '.'


Verification for us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753 --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - Existence of the claims in the transparency log was verified offline
  - Any certificates were verified against the Fulcio roots.
Certificate subject:  cosign@cosign-test-383122.iam.gserviceaccount.com
Certificate issuer URL:  https://accounts.google.com
{
  "payloadType": "application/vnd.in-toto+json",
  "payload": "eyJfdHlwZSI6Imh0dHBzOi8vaW4tdG90by5pby9TdGF0ZW1lbnQvdjAuMSIsInByZWRpY2F0ZVR5cGUiOiJjb3NpZ24uc2lnc3RvcmUuZGV2L2F0dGVzdGF0aW9uL3YxIiwic3ViamVjdCI6W3sibmFtZSI6InVzLWNlbnRyYWwxLWRvY2tlci5wa2cuZGV2L2Nvc2lnbi10ZXN0LTM4MzEyMi9yZXBvMS9zZWN1cmVidWlsZCIsImRpZ2VzdCI6eyJzaGEyNTYiOiI4M2FiMmJhNjY4OTcxM2YyZDY4MTA0Y2QyMDhmZWFkZmViZGQ2YmM4ODFjNDU1ZGNiNTVkMmI0NWFjM2EwNzUzIn19XSwicHJlZGljYXRlIjp7IkRhdGEiOiJ7IFwicHJvamVjdGlkXCI6IFwiY29zaWduLXRlc3QtMzgzMTIyXCIsIFwiYnVpbGRpZFwiOiBcIjBhNzc0MzYwLWQ1N2UtNDFhZi1hZjhiLTc2ZGQ0MTA5YzQ3Y1wiLCBcImZvb1wiOlwiYmFyXCIsIFwiY29tbWl0c2hhXCI6IFwiNTJkYzhjMTk3OWQ3ZTZiNTZhNWEyNTNkYmQ3OTAyODg0Mjc1MmIwOFwiIH0iLCJUaW1lc3RhbXAiOiIyMDIzLTA0LTA4VDIyOjMyOjM0WiJ9fQ==",
  "signatures": [
    {
      "keyid": "",
      "sig": "MEUCIQCVzyk9aAXRY0rlk+D7ad8MN0pGBeXQqmGJd+xl9QRMDAIgC0DoUPsUVK2IrErtiH4XWJktJC1WxypnMA78ByvjDoY="
    }
  ]
}
```

### Verify with dockerhub image

I've also uploaded this sample to dockerhub so you can verify without container registry:

```bash
cosign sign --annotations=key1=value1 \
  --key gcpkms://projects/$PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 \
      docker.io/salrashid123/securebuild:server

cosign verify --key cert/kms_pub.pem   \
    docker.io/salrashid123/securebuild:server@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753  | jq '.'


COSIGN_EXPERIMENTAL=1 cosign attest \
  --key gcpkms://projects/$PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 \
  -f \
  --predicate=predicate.json  docker.io/salrashid123/securebuild:server


COSIGN_EXPERIMENTAL=1 cosign verify-attestation  --key cert/kms_pub.pem --policy policy.rego    \
       docker.io/salrashid123/securebuild:server@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753  | jq '.'
```

### Cosign and Rekor APIs

You can use the cosign and Rekor APIs as well.

The `client/main.go` sample application iterates over the signatures and attestations for the image hash in this repo.  


By default, it will scan the dockerhub registry but you can alter it to use your GCP Artifiact Registry.  


```bash
cd client/
go run main.go 

>>>>>>>>>> Search rekor <<<<<<<<<<
LogIndex 17477093
 UUID c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d
 Entry API Version 0.0.1
 Kind: intoto
 PublicKey:
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEPFwNMvKlujD9GHUG2jN6hSZJjatd
RieR83oRE901m/Bc6iNM5QfaaKC5roUPf7LL7DggnTdJ1aLnJZw5qViAzA==
-----END PUBLIC KEY-----
 rekor logentry inclustion verified
LogIndex 17475497
 UUID c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d
 Entry API Version 0.0.1
 Kind: intoto
 PublicKey:
-----BEGIN CERTIFICATE-----
MIIC6TCCAnCgAwIBAgIUKFeyy8/cGb0fPsy3UOX4gK1d1YgwCgYIKoZIzj0EAwMw
NzEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MR4wHAYDVQQDExVzaWdzdG9yZS1pbnRl
cm1lZGlhdGUwHhcNMjMwNDA4MjIzMjM0WhcNMjMwNDA4MjI0MjM0WjAAMFkwEwYH
KoZIzj0CAQYIKoZIzj0DAQcDQgAEewrPTefh0V011mk7wfDv5bClQ13irAu1cjP3
foGWKWqUy6CzN25NMoE73HZLibH+F5KKnWlEaXZoSwdyBYMt9aOCAY8wggGLMA4G
A1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAdBgNVHQ4EFgQU2Gic
3T1X1AvniYTUoSdrf9RN0bAwHwYDVR0jBBgwFoAU39Ppz1YkEZb5qNjpKFWixi4Y
ZD8wPwYDVR0RAQH/BDUwM4ExY29zaWduQGNvc2lnbi10ZXN0LTM4MzEyMi5pYW0u
Z3NlcnZpY2VhY2NvdW50LmNvbTApBgorBgEEAYO/MAEBBBtodHRwczovL2FjY291
bnRzLmdvb2dsZS5jb20wKwYKKwYBBAGDvzABCAQdDBtodHRwczovL2FjY291bnRz
Lmdvb2dsZS5jb20wgYoGCisGAQQB1nkCBAIEfAR6AHgAdgDdPTBqxscRMmMZHhyZ
ZzcCokpeuN48rf+HinKALynujgAAAYdi//VAAAAEAwBHMEUCIFGJEFbZikCDOqh0
W8xriW0U/Z5VoLv+fIdXy2i0tZfBAiEA3cQ2yBG23T23W1ZE+03bSGnxRaSqWFl+
PGAVyDKQpxEwCgYIKoZIzj0EAwMDZwAwZAIwa2Cefp0fSXJ7NMx7LRKDyaFYDIfW
5jdTisk6fErBKKD6siuNPwCJ5IfJySFNmuoZAjAzBWOsaQljvWCr+IxKLF0OTdKm
hAIwKqQBDAs+fwT93lvUAW33hSP+4tNy0/9eBJI=
-----END CERTIFICATE----
 rekor logentry inclustion verified
LogIndex 17474990
 UUID c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d
 Entry API Version 0.0.1
 Kind: intoto
 PublicKey:
-----BEGIN CERTIFICATE-----
MIIC6zCCAnGgAwIBAgIUOcYlHoxtkY8yWq0H666BLcJ50+MwCgYIKoZIzj0EAwMw
NzEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MR4wHAYDVQQDExVzaWdzdG9yZS1pbnRl
cm1lZGlhdGUwHhcNMjMwNDA4MjIyMDQxWhcNMjMwNDA4MjIzMDQxWjAAMFkwEwYH
KoZIzj0CAQYIKoZIzj0DAQcDQgAEIIoO7NyVM5x/yOzIZuMt2ucAh8nnhrt3OtQs
iDp03nY2Wh+wrAjaepMqeMxAtqtnluEa+1nOFFNjESd5WcsG36OCAZAwggGMMA4G
A1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAdBgNVHQ4EFgQUU/ZJ
1BsK9RDRWKDxKAfIzWgk07EwHwYDVR0jBBgwFoAU39Ppz1YkEZb5qNjpKFWixi4Y
ZD8wQAYDVR0RAQH/BDYwNIEyY29zaWduQG1pbmVyYWwtbWludXRpYS04MjAuaWFt
LmdzZXJ2aWNlYWNjb3VudC5jb20wKQYKKwYBBAGDvzABAQQbaHR0cHM6Ly9hY2Nv
dW50cy5nb29nbGUuY29tMCsGCisGAQQBg78wAQgEHQwbaHR0cHM6Ly9hY2NvdW50
cy5nb29nbGUuY29tMIGKBgorBgEEAdZ5AgQCBHwEegB4AHYA3T0wasbHETJjGR4c
mWc3AqJKXrjePK3/h4pygC8p7o4AAAGHYvUUNAAABAMARzBFAiBODxOKyjHWSmeU
rxHorSrRN/CL5Krae+eoy4NFFe1mcAIhAL71HXWFI4I7HU+RuxnbC8NNUyco6SX7
nfvMPxt9IIdoMAoGCCqGSM49BAMDA2gAMGUCMQDpC7ME7+R+Zas9fH+F6Egi0yN5
C82RTIWHSIQSBx/cyqheDfJzFPUY6N+N8raM4i4CMHWNA/egoeaFCFUA9qcOuTXc
nLPaKtm2nIG8ah55fERrf/4muJqXxqfZ/B2nF6WLmA==
-----END CERTIFICATE-----

 rekor logentry inclustion verified
LogIndex 17204909
 UUID c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d
 Entry API Version 0.0.1
 Kind: intoto
 PublicKey:
-----BEGIN CERTIFICATE-----
MIIC7DCCAnGgAwIBAgIUPjIAKRyOCFfDKtitS+W13np1oR0wCgYIKoZIzj0EAwMw
NzEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MR4wHAYDVQQDExVzaWdzdG9yZS1pbnRl
cm1lZGlhdGUwHhcNMjMwNDA1MTkwNzE3WhcNMjMwNDA1MTkxNzE3WjAAMFkwEwYH
KoZIzj0CAQYIKoZIzj0DAQcDQgAE21t2Bxpm/DnAtrW9mwM6FpXWLdT8Y++HhSHP
FoXZX34x0Zll7UjZ1rbYGxsxDksNh3T49axrP/hSkxk7CwWV06OCAZAwggGMMA4G
A1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAdBgNVHQ4EFgQUl/4/
awtloWkDKB8oLYj8V9b5KJ4wHwYDVR0jBBgwFoAU39Ppz1YkEZb5qNjpKFWixi4Y
ZD8wQAYDVR0RAQH/BDYwNIEyY29zaWduQG1pbmVyYWwtbWludXRpYS04MjAuaWFt
LmdzZXJ2aWNlYWNjb3VudC5jb20wKQYKKwYBBAGDvzABAQQbaHR0cHM6Ly9hY2Nv
dW50cy5nb29nbGUuY29tMCsGCisGAQQBg78wAQgEHQwbaHR0cHM6Ly9hY2NvdW50
cy5nb29nbGUuY29tMIGKBgorBgEEAdZ5AgQCBHwEegB4AHYA3T0wasbHETJjGR4c
mWc3AqJKXrjePK3/h4pygC8p7o4AAAGHUtDxogAABAMARzBFAiAy49G754A3ZMOm
fydikVN5k9ycBCMg0EoXH1LBaWeBswIhAKrOWlQZLktQTfnjAOBi6BAae6snCBMq
M3wYjB3PAz9lMAoGCCqGSM49BAMDA2kAMGYCMQDk34fHpWKN1PhkGCXzVaDi950Q
FatRjfnuyOcfaZvOeSWVkIFdetmZ8WC59mOVyNwCMQDNPsLXTi0/JonZAj0xYzBQ
vWBbfAHhOB4gUIs7qafmuItVdlAj9MzmefFXwyc4nyY=
-----END CERTIFICATE-----

 rekor logentry inclustion verified
LogIndex 4886588
 UUID c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d
 Entry API Version 0.0.1
 Kind: intoto
 PublicKey:
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEjNbDUDgtOGPkkBYbY+m8O95e+WQJ
NuFKm46ooRBeRw/92iTzPmHmY4/fF+XeiMIEmlNim0WkhHfxpWFSL48sog==
-----END PUBLIC KEY-----
 rekor logentry inclustion verified
LogIndex 4884684
 UUID c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d
 Entry API Version 0.0.1
 Kind: intoto
 PublicKey:
-----BEGIN CERTIFICATE-----
MIICvTCCAkOgAwIBAgIUPFs1vvKZpTTL2QoBRMMMoBenoswwCgYIKoZIzj0EAwMw
NzEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MR4wHAYDVQQDExVzaWdzdG9yZS1pbnRl
cm1lZGlhdGUwHhcNMjIxMDExMDk1MjI5WhcNMjIxMDExMTAwMjI5WjAAMFkwEwYH
KoZIzj0CAQYIKoZIzj0DAQcDQgAEUrjCJ1a21A5e+8T+P1Sb8tB4kz6t/XWQfpaT
+LuEvmIqKseMPiNCXlQ0cuJDMBCPVOvqzGGPXl2zwQSBZFtOsKOCAWIwggFeMA4G
A1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAdBgNVHQ4EFgQUmBj5
scvtxYDcrdXOCzoCdyptj50wHwYDVR0jBBgwFoAU39Ppz1YkEZb5qNjpKFWixi4Y
ZD8wPwYDVR0RAQH/BDUwM4ExY29zaWduQGNvc2lnbi10ZXN0LTM2NTIwOS5pYW0u
Z3NlcnZpY2VhY2NvdW50LmNvbTApBgorBgEEAYO/MAEBBBtodHRwczovL2FjY291
bnRzLmdvb2dsZS5jb20wgYoGCisGAQQB1nkCBAIEfAR6AHgAdgAIYJLwKFL/aEXR
0WsnhJxFZxisFj3DONJt5rwiBjZvcgAAAYPGdcIHAAAEAwBHMEUCIERwE0LQXDJk
7Geiq4D9vpnscmhL5I/icEun+kP5/pxKAiEAuxJSN4zPlYqd7hbdItcuELj3b/jc
718pD/y6oo5y7lUwCgYIKoZIzj0EAwMDaAAwZQIxAMt0QpEeQrjooEx7LKLUtSja
MCCzqc+Qkd1i2DxeT6Nob6oqo9izwOUEpODgkPrd0gIwbnyLmdDetq7jSrK94ep4
0no7Bcns7404NXcvXZMGQi64pmTHUmc0uQIxr00VSGVf
-----END CERTIFICATE-----
 rekor logentry inclustion verified
LogIndex 3944855
 UUID c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d
 Entry API Version 0.0.1
 Kind: intoto
 PublicKey:
-----BEGIN CERTIFICATE-----
MIICvTCCAkOgAwIBAgIUJLAclsx9jP0T5ZgaZJ3fBtYAE8YwCgYIKoZIzj0EAwMw
NzEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MR4wHAYDVQQDExVzaWdzdG9yZS1pbnRl
cm1lZGlhdGUwHhcNMjIwOTI1MTQwNDUwWhcNMjIwOTI1MTQxNDUwWjAAMFkwEwYH
KoZIzj0CAQYIKoZIzj0DAQcDQgAEKA1B7l3Nx0to+Z0DtduiXJOIrI4r7L1ear35
ocUFYjPafP5tOB/5uXdsgRB9uVlAsQUa1B69vNbrAAv8kmEUc6OCAWIwggFeMA4G
A1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAdBgNVHQ4EFgQUIHCP
oQfc58fI+RugfR5WDQXIbwkwHwYDVR0jBBgwFoAU39Ppz1YkEZb5qNjpKFWixi4Y
ZD8wPwYDVR0RAQH/BDUwM4ExY29zaWduQGNvc2lnbi10ZXN0LTM2MzYxNS5pYW0u
Z3NlcnZpY2VhY2NvdW50LmNvbTApBgorBgEEAYO/MAEBBBtodHRwczovL2FjY291
bnRzLmdvb2dsZS5jb20wgYoGCisGAQQB1nkCBAIEfAR6AHgAdgAIYJLwKFL/aEXR
0WsnhJxFZxisFj3DONJt5rwiBjZvcgAAAYN09wtZAAAEAwBHMEUCIQDckcOI/N44
4/yIyX8xc6z8J9K+OMBgnaT/I5KWGcAmJgIgaozC85AeMSBz/coNS/IcP51mTwM0
gZSS45I/D8Aw+kQwCgYIKoZIzj0EAwMDaAAwZQIxALFjPTLIY5sQAeMAf0MSF51P
1YUWXA8tQAAJEaBAyrJKiwzFxz+sWDLNc6oE6+/OxQIwdn0R8z9CFyOzdVNpEY94
dDqYllqNlcSL52TafK0aaTgigptedOftbttvaF99wnur
-----END CERTIFICATE-----
 rekor logentry inclustion verified
LogIndex 3942848
 UUID c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d
 Entry API Version 0.0.1
 Kind: intoto
 PublicKey:
-----BEGIN CERTIFICATE-----
MIICvDCCAkOgAwIBAgIUCRasP2dIpyOLnsxBZg+JrgtdVQcwCgYIKoZIzj0EAwMw
NzEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MR4wHAYDVQQDExVzaWdzdG9yZS1pbnRl
cm1lZGlhdGUwHhcNMjIwOTI1MTMxNjM3WhcNMjIwOTI1MTMyNjM3WjAAMFkwEwYH
KoZIzj0CAQYIKoZIzj0DAQcDQgAErNXX7JaOVfyxuDAlOul44zjlBODELx+CF64B
vp5/LXMS9NDU5eGlfDhw1dmtuHeS/HUyILFWSe/dUBv7FAcNdqOCAWIwggFeMA4G
A1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAdBgNVHQ4EFgQUbxul
eomPawZobo7gdrFGAWaajJ0wHwYDVR0jBBgwFoAU39Ppz1YkEZb5qNjpKFWixi4Y
ZD8wPwYDVR0RAQH/BDUwM4ExY29zaWduQGNvc2lnbi10ZXN0LTM2MzYxMy5pYW0u
Z3NlcnZpY2VhY2NvdW50LmNvbTApBgorBgEEAYO/MAEBBBtodHRwczovL2FjY291
bnRzLmdvb2dsZS5jb20wgYoGCisGAQQB1nkCBAIEfAR6AHgAdgAIYJLwKFL/aEXR
0WsnhJxFZxisFj3DONJt5rwiBjZvcgAAAYN0yuYwAAAEAwBHMEUCIQDPI8xna678
YQh1zFyv0QbG298F04cBdFRpO7B1E5LupQIgAdalXGD2l5LlY3TDQx8epodOT4Hk
It8Iv/Cjm6RzYWEwCgYIKoZIzj0EAwMDZwAwZAIwJ5Ruij7ukyMlBdDvM7tEAX81
NjbUGPuc2+3U/38fYOgTV3UmuWgsERCIYndFjJtWAjBXA9Ilo0+odoMH/zGRC6Pp
kf4ghWWMEyXndi2W/zXeuCKZqc5fpEyxD7oqNnyM+7A=
-----END CERTIFICATE-----
 rekor logentry inclustion verified
>>>>>>>>>> Verifying Image Signatures using provided PublicKey <<<<<<<<<<
Verified signature MEQCIC/pp/wJIV3gmOhxdLMi8UcIwC47U8WFVicMcntF2ZtQAiAl6kYqifpQFqPEFsUTl5cTf+hFuuLlGzeWosBnLwwZeA==
  Image Ref {sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753}

```


### Sign without upload to registry


The following will sign an image with a key and verify with the signatuere provided inline (`--signature`)

```bash
export IMAGE=docker.io/salrashid123/securebuild:server
export IMAGE_DIGEST=$IMAGE@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753

gcloud kms keys versions get-public-key 1  \
   --key=key1 --keyring=cosignkr  \
    --location=global --output-file=/tmp/kms_pub.pem


### sign
$ cosign sign \
  --key gcpkms://projects/$PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 \
  --upload=false $IMAGE_DIGEST --no-tlog-upload=true --output-signature sig.txt

$ cat sig.txt 
MEQCICyVTj5NY++6JuWpRf2OlzVFwD3C1qfpZDsjia/gBooxAiBw7DMxWXDivBEh3rLifU1jKZstAUPwilbsP3fjut2yAQ==

cosign verify  --key /tmp/kms_pub.pem $IMAGE --signature sig.txt | jq '.'

## if you want it added to the registry, the *owner* of that registry must attach the signature
cosign attach signature --signature `cat sig.txt` $IMAGE_DIGEST

### attest

cosign attest \
  --key gcpkms://projects/$PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 \
  -f --no-tlog-upload=true --no-upload=true \
  --predicate=predicate.json  $IMAGE_DIGEST --output-file=/tmp/attest.txt


cat /tmp/attest.txt  | jq '.'
{
  "payloadType": "application/vnd.in-toto+json",
  "payload": "eyJfdHlwZSI6Imh0dHBzOi8vaW4tdG90by5pby9TdGF0ZW1lbnQvdjAuMSIsInByZWRpY2F0ZVR5cGUiOiJjb3NpZ24uc2lnc3RvcmUuZGV2L2F0dGVzdGF0aW9uL3YxIiwic3ViamVjdCI6W3sibmFtZSI6ImluZGV4LmRvY2tlci5pby9zYWxyYXNoaWQxMjMvc2VjdXJlYnVpbGQiLCJkaWdlc3QiOnsic2hhMjU2IjoiODNhYjJiYTY2ODk3MTNmMmQ2ODEwNGNkMjA4ZmVhZGZlYmRkNmJjODgxYzQ1NWRjYjU1ZDJiNDVhYzNhMDc1MyJ9fV0sInByZWRpY2F0ZSI6eyJEYXRhIjoieyBcbiAgICBcInByb2plY3RpZFwiOiBcIiRQUk9KRUNUX0lEXCIsIFxuICAgIFwiYnVpbGRpZFwiOiBcIiRCVUlMRF9JRFwiLCBcbiAgICBcImZvb1wiOiBcImJhclwiLCBcbiAgICBcImNvbW1pdHNoYVwiOiBcImZvb1wiXG59IiwiVGltZXN0YW1wIjoiMjAyMy0wNC0wOFQyMzoxNjo1OFoifX0=",
  "signatures": [
    {
      "keyid": "",
      "sig": "MEUCIQCVXHZwaZdYBGsUjVyhn0wBnjv5IWS3QJLiNqbvRoypngIgMUF3wdLJVZ5pZPSQvHEBYBFGFXbIj0+xxlkxzW6zVME="
    }
  ]
}

```

### Sign offline and attach

The following will allow two different signer to create signatures using their own keys and then the repo owner can `attach` the signatures to the registry.  

```bash
export IMAGE=us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild:server
export IMAGE_DIGEST=us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753

### Create a signer
cosign generate-key-pair

mv cosign.key c1.key
mv cosign.pub c1.pub

cosign sign \
  --key c1.key \
  --upload=false $IMAGE_DIGEST --no-tlog-upload=true --output-signature sig.txt

cosign verify  --key c1.pub $IMAGE --signature sig.txt | jq '.'

$ cosign tree us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753
  📦 Supply Chain Security Related artifacts for an image: us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753
  └── 💾 Attestations for an image tag: us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild:sha256-83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753.att
    ├── 🍒 sha256:400b85ee2faf8e939ca92644dab7fcb69680729fcdf18c6d5798823e694bdeb8
    └── 🍒 sha256:b13908ecec6666c94a7ed985f51dab3dd42f24fa64650f247db270f9d89f2d79
  └── 🔐 Signatures for an image tag: us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild:sha256-83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753.sig
    ├── 🍒 sha256:02277181cf8f82a917f1f7348d2838ef91261739a1f2cd315d35b4da5a7655ce
    └── 🍒 sha256:02277181cf8f82a917f1f7348d2838ef91261739a1f2cd315d35b4da5a7655ce
  └── 📦 SBOMs for an image tag: us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild:sha256-83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753.sbom
    └── 🍒 sha256:65178140d352ed9fd6a5f2e3d1a126497dab116f1ceb8c5fe5685516011e07b5

  # attach as repo owner
cosign attach signature --signature `cat sig.txt` $IMAGE_DIGEST


$ cosign tree us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753
  📦 Supply Chain Security Related artifacts for an image: us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753
  └── 💾 Attestations for an image tag: us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild:sha256-83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753.att
    ├── 🍒 sha256:400b85ee2faf8e939ca92644dab7fcb69680729fcdf18c6d5798823e694bdeb8
    └── 🍒 sha256:b13908ecec6666c94a7ed985f51dab3dd42f24fa64650f247db270f9d89f2d79
  └── 🔐 Signatures for an image tag: us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild:sha256-83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753.sig
    ├── 🍒 sha256:02277181cf8f82a917f1f7348d2838ef91261739a1f2cd315d35b4da5a7655ce
    ├── 🍒 sha256:02277181cf8f82a917f1f7348d2838ef91261739a1f2cd315d35b4da5a7655ce
    └── 🍒 sha256:15e121a0a6228ef2dc88ec13a865c3963fc238c64dc4e09b48edd80d573dadcb
  └── 📦 SBOMs for an image tag: us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild:sha256-83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753.sbom
    └── 🍒 sha256:65178140d352ed9fd6a5f2e3d1a126497dab116f1ceb8c5fe5685516011e07b5


### Create a new signer

cosign generate-key-pair

mv cosign.key c2.key
mv cosign.pub c2.pub

cosign sign \
  --key c2.key \
  --upload=false $IMAGE_DIGEST --no-tlog-upload=true --output-signature sig.txt

cosign verify  --key c2.pub $IMAGE --signature sig.txt | jq '.'

# attach as repo owner
cosign attach signature --signature `cat sig.txt` $IMAGE_DIGEST

📦 Supply Chain Security Related artifacts for an image: us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753
└── 💾 Attestations for an image tag: us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild:sha256-83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753.att
   ├── 🍒 sha256:400b85ee2faf8e939ca92644dab7fcb69680729fcdf18c6d5798823e694bdeb8
   └── 🍒 sha256:b13908ecec6666c94a7ed985f51dab3dd42f24fa64650f247db270f9d89f2d79
└── 🔐 Signatures for an image tag: us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild:sha256-83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753.sig
   ├── 🍒 sha256:02277181cf8f82a917f1f7348d2838ef91261739a1f2cd315d35b4da5a7655ce
   ├── 🍒 sha256:02277181cf8f82a917f1f7348d2838ef91261739a1f2cd315d35b4da5a7655ce
   ├── 🍒 sha256:15e121a0a6228ef2dc88ec13a865c3963fc238c64dc4e09b48edd80d573dadcb
   └── 🍒 sha256:15e121a0a6228ef2dc88ec13a865c3963fc238c64dc4e09b48edd80d573dadcb
└── 📦 SBOMs for an image tag: us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild:sha256-83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753.sbom
   └── 🍒 sha256:65178140d352ed9fd6a5f2e3d1a126497dab116f1ceb8c5fe5685516011e07b5

```


### Verify the image `sbom`

```bash
# cosign verify --key /tmp/kms_pub.pem --attachment=sbom          us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753  | jq '.'

Verification for us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild:sha256-83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753.sbom --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - The signatures were verified against the specified public key
[
  {
    "critical": {
      "identity": {
        "docker-reference": "us-central1-docker.pkg.dev/cosign-test-383122/repo1/securebuild"
      },
      "image": {
        "docker-manifest-digest": "sha256:85313f9a6c496176b3ae16c8602b396574f0bb47c43b395afa17865d9437d997"
      },
      "type": "cosign container image signature"
    },
    "optional": {
      "commit_sha": "52dc8c1979d7e6b56a5a253dbd79028842752b08"
    }
  }
]
```

---

thats as much as i know about this at the time of writing..

---
