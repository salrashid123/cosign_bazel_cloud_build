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
* [Deterministic container hashes and container signing using Cosign, Kaniko and Google Cloud Build](https://github.com/salrashid123/cosign_kaniko_cloud_build)
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

* `securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00`

![images/build.png](images/build.png)

#### Push image to registry

This pushes the image to [google artifact registry](https://cloud.google.com/artifact-registry).  We are using [skopeo](https://github.com/containers/skopeo) with the [`--preserve-digests`] flag.  This will give the image hash we'd expect

You can optionally push to dockerhub if you want using [KMS based secrets](https://cloud.google.com/build/docs/securing-builds/use-encrypted-credentials#configuring_builds_to_use_encrypted_data) in cloud build 

#### Create attestations attributes

This step will issue a statement that includes attestation attributes users can inject into the pipeline the verifier can use. See [Verify Attestations](https://docs.sigstore.dev/cosign/verify/).

There are two attestation: 1. container sbom genreated by `syft` and 2. a plain application attestation.

For 1, we used syft to generate the containers' `cyclonedx` attestation

For 2, the attestation verification predicate includes some info from the build like the buildID and even the repo commithash.

Someone who wants to verify any [in-toto attestation](https://docs.sigstore.dev/cosign/attestation/) can use these values. This repo just adds some basic stuff like the `projectID`,  `buildID` and `commitsha` (in our case, its `71d4d7251cc8f993ad4094ddda33a65fee9c4df4`):


```json
{ "projectid": "$PROJECT_ID", "buildid": "$BUILD_ID", "foo":"bar", "commitsha": "$COMMIT_SHA" }
```

#### Sign image using KMS based keys

This step uses the KMS key to `cosign` the image


#### Apply attestations using KMS

This issues attestation signature using some predicates we wrote to file during the build.

You can define any claims here..i just happen to use the commit hash for the source and some random stuff.

we also attested the syft container sbom we created earlier

#### Sign image using OIDC tokens

This step will use the service accounts OIDC token sign using [Fulcio](https://docs.sigstore.dev/fulcio/oidc-in-fulcio) for the syft sbom:

#### Apply attestations using OIDC tokens

This will issue signed attestations using the OIDC token signing for fulcio

![images/attest_oidc.png](images/attest_oidc.png)

for the syft sbom:

![images/attest_packages_oidc.png](images/attest_packages_oidc.png)
#### Use Syft to generate image sbom

Generate the container image's sbom

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
gcloud iam service-accounts create cosign-svc-account

# allow 'self impersonation' for cloud build service account
gcloud iam service-accounts add-iam-policy-binding cosign-svc-account@$PROJECT_ID.iam.gserviceaccount.com \
    --role roles/iam.serviceAccountTokenCreator \
    --member "serviceAccount:cosign-svc-account@$PROJECT_ID.iam.gserviceaccount.com"

# allow cloud build to write logs
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member=serviceAccount:cosign-svc-account@$PROJECT_ID.iam.gserviceaccount.com  \
  --role=roles/logging.logWriter

# allow cloud build write access to artifact registry
gcloud artifacts repositories add-iam-policy-binding repo1 \
    --location=us-central1  \
    --member=serviceAccount:cosign-svc-account@$PROJECT_ID.iam.gserviceaccount.com \
    --role=roles/artifactregistry.writer

# allow cloud build access to list KMS keys
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member=serviceAccount:cosign-svc-account@$PROJECT_ID.iam.gserviceaccount.com  \
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
    --member=serviceAccount:cosign-svc-account@$PROJECT_ID.iam.gserviceaccount.com \
    --role=roles/cloudkms.signer

# allow current gcloud user to view the public key
gcloud kms keys add-iam-policy-binding key1 \
    --keyring=cosignkr --location=global \
    --member=serviceAccount:cosign-svc-account@$PROJECT_ID.iam.gserviceaccount.com  \
    --role=roles/cloudkms.publicKeyViewer

# create a temp bucket for cloud build and allow cloud build permissions to use it
gsutil mb gs://$PROJECT_ID\_cloudbuild
gsutil iam ch serviceAccount:cosign-svc-account@$PROJECT_ID.iam.gserviceaccount.com:objectAdmin gs://$PROJECT_ID\_cloudbuild
```

### Build image

```bash
# to build directly
# cd /app
# gcloud beta builds submit --config=cloudbuild.yaml --machine-type=n1-highcpu-32

# to build via commit (recommended)
gcloud source repos create cosign-repo

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member=serviceAccount:cosign-svc-account@$PROJECT_ID.iam.gserviceaccount.com \
  --role=roles/source.reader

gcloud source repos clone cosign-repo
cd cosign-repo
cp -R ../app/* .


# bazel in docker locally
# docker run   -e USER="$(id -u)" \
#    -v `pwd`:/src/workspace   -v /tmp/build_output:/tmp/build_output  \
#     -v /var/run/docker.sock:/var/run/docker.sock   -w /src/workspace  \
#     gcr.io/cloud-builders/bazel@sha256:f00a985c3196cc58819b6f7e8e40353273bc20e8f24b54d9c92d5279bb5b3fad  \
#      --output_user_root=/tmp/build_output   run  --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64 :server
#
# using https://github.com/containers/skopeo
# skopeo inspect --format "{{.Name}}@{{.Digest}}"  docker-daemon:us-central1-docker.pkg.dev/builder-project/repo1/securebuild-bazel:server
# ## gives
#    us-central1-docker.pkg.dev/builder-project/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00


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
   --service-account=projects/$PROJECT_ID/serviceAccounts/cosign-svc-account@$PROJECT_ID.iam.gserviceaccount.com 

# now trigger
gcloud alpha builds triggers run cosign-trigger --branch=main
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
   us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00  | jq '.'

# or by api
# cosign verify --key gcpkms://projects/$PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 \
#       us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00 | jq '.'
```

Note this gives 

```text
Verification for us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00 --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - The signatures were verified against the specified public key
[
  {
    "critical": {
      "identity": {
        "docker-reference": "us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel"
      },
      "image": {
        "docker-manifest-digest": "sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00"
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
COSIGN_EXPERIMENTAL=1  cosign verify  us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00 | jq '.'
```

gives

```text
Verification for us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00 --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - Existence of the claims in the transparency log was verified offline
  - Any certificates were verified against the Fulcio roots.
[
  {
    "critical": {
      "identity": {
        "docker-reference": "us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel"
      },
      "image": {
        "docker-manifest-digest": "sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00"
      },
      "type": "cosign container image signature"
    },
    "optional": {
      "1.3.6.1.4.1.57264.1.1": "https://accounts.google.com",
      "Bundle": {
        "SignedEntryTimestamp": "MEUCIF0BDv4ma9gi2t7P15xlAnmRAzIqDGuDjc+myBQugfuVAiEA+fDQZRSwHvOPxrRGPxxah3l28eaEUIe6SqMzVw/qXtY=",
        "Payload": {
          "body": "eyJhcGlWZXJzaW9uIjoiMC4wLjEiLCJraW5kIjoiaGFzaGVkcmVrb3JkIiwic3BlYyI6eyJkYXRhIjp7Imhhc2giOnsiYWxnb3JpdGhtIjoic2hhMjU2IiwidmFsdWUiOiJjYjMxNGRkYzk3ODNjMTgxMmFmZTgyMzVmZDJiN2Q2MzIzZTQwOGNjNTNkMDZhNjljY2JhNjA0NTEwYTVjYTg1In19LCJzaWduYXR1cmUiOnsiY29udGVudCI6Ik1FWUNJUUN3WW4relNaUHh3NjFIZGtUb2RDMFVoNWtzUDErYzJyYzc4SVV6WjhGY3BRSWhBSkVMUG9kd3BSaDRXZFpVdXRhV0JwNmZGYUxUL3pLVlN2aDlWMU1LVFZMLyIsInB1YmxpY0tleSI6eyJjb250ZW50IjoiTFMwdExTMUNSVWRKVGlCRFJWSlVTVVpKUTBGVVJTMHRMUzB0Q2sxSlNVTXJSRU5EUVc0MlowRjNTVUpCWjBsVlFXNURaa0Z3WkdaQlFqSkJVMDFzVGpObEszSTVXVFp5VjFodmQwTm5XVWxMYjFwSmVtb3dSVUYzVFhjS1RucEZWazFDVFVkQk1WVkZRMmhOVFdNeWJHNWpNMUoyWTIxVmRWcEhWakpOVWpSM1NFRlpSRlpSVVVSRmVGWjZZVmRrZW1SSE9YbGFVekZ3WW01U2JBcGpiVEZzV2tkc2FHUkhWWGRJYUdOT1RXcE5kMDVVUVROTlZFMTRUbFJGZVZkb1kwNU5hazEzVGxSQk0wMVVUWGxPVkVWNVYycEJRVTFHYTNkRmQxbElDa3R2V2tsNmFqQkRRVkZaU1V0dldrbDZhakJFUVZGalJGRm5RVVV3UW5WR1V6ZGFUbFZSZEZwVGQwbExaRWRYWlhBMVUwTlNjblkzUm1Wc1dtWXpSM2dLTURONmFWRmhURE5RYm1Gdk9FSkNXRTExVlROTlkyWlZVV1JYZEV4SGRFaHhWbTQzYm1aME9VSndZM2xSWkhNNVJtRlBRMEZhTUhkblowZGFUVUUwUndwQk1WVmtSSGRGUWk5M1VVVkJkMGxJWjBSQlZFSm5UbFpJVTFWRlJFUkJTMEpuWjNKQ1owVkdRbEZqUkVGNlFXUkNaMDVXU0ZFMFJVWm5VVlZFVVRWRUNqSnFSbmN5TjFsblQwTm5lRmhVZFdGWE5sZGxjVzlWZDBoM1dVUldVakJxUWtKbmQwWnZRVlV6T1ZCd2VqRlphMFZhWWpWeFRtcHdTMFpYYVhocE5Ga0tXa1E0ZDFSQldVUldVakJTUVZGSUwwSkZTWGRSU1VVcldUSTVlbUZYWkhWTVdFNHlXWGt4YUZreVRuWmtWelV3VVVjeGNHSnRWbmxaVjNkMFlsZHNkUXBrV0ZKd1dWTXdORTFxUVhWaFYwWjBURzFrZWxwWVNqSmhWMDVzV1ZkT2FtSXpWblZrUXpWcVlqSXdkMHRSV1V0TGQxbENRa0ZIUkhaNlFVSkJVVkZpQ21GSVVqQmpTRTAyVEhrNWFGa3lUblprVnpVd1kzazFibUl5T1c1aVIxVjFXVEk1ZEUxRGMwZERhWE5IUVZGUlFtYzNPSGRCVVdkRlNGRjNZbUZJVWpBS1kwaE5Oa3g1T1doWk1rNTJaRmMxTUdONU5XNWlNamx1WWtkVmRWa3lPWFJOU1VkTVFtZHZja0puUlVWQlpGbzFRV2RSUTBKSU1FVmxkMEkxUVVoalFRb3pWREIzWVhOaVNFVlVTbXBIVWpSamJWZGpNMEZ4U2t0WWNtcGxVRXN6TDJnMGNIbG5Remh3TjI4MFFVRkJSMGc1Ykc5aE1rRkJRVUpCVFVGVFJFSkhDa0ZwUlVFMFJDOWhZVWRzY1VVM0syaDRWa2MyUkc5TlMxbDBhakJwUkM5bVYwaHRaVVpGWkRSTk1XZFZiVE56UTBsUlJERTRiVEJKSzJ0NU9EbGlNazBLV2tkcGIwcDFjMjlqVkVWVE9EQkpkREp3VkRaRVEwOVJSSEZZWlU1VVFVdENaMmR4YUd0cVQxQlJVVVJCZDA1dlFVUkNiRUZxUVd0VGJTdDVWbXBXZVFwV2IxTkNPRGRDUjBkNFJ6VnNhbEV6WVZkUldsVmtiRFU1TW1KVFNtbzJTbVpWU1RONGNYQkNUa2xMTjJzeFFsTmpNV2hOYkZCVlEwMVJRMlUyYWk5SkNtOUVWMmxDT1c5SmVWcENWWEZHYWpKRGVtaFJORU5wUzFGYVlXcFZUbWxRYTBSUWJVWnlPRGhzTnpSNEwyRktjVk5vYjBKNlZraDFPRzUzUFFvdExTMHRMVVZPUkNCRFJWSlVTVVpKUTBGVVJTMHRMUzB0Q2c9PSJ9fX19",
          "integratedTime": 1683465314,
          "logIndex": 19941761,
          "logID": "c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d"
        }
      },
      "Issuer": "https://accounts.google.com",
      "Subject": "cosign-svc-account@$PROJECT_ID.iam.gserviceaccount.com",
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
        "value": "cb314ddc9783c1812afe8235fd2b7d6323e408cc53d06a69ccba604510a5ca85"
      }
    },
    "signature": {
      "content": "MEYCIQCwYn+zSZPxw61HdkTodC0Uh5ksP1+c2rc78IUzZ8FcpQIhAJELPodwpRh4WdZUutaWBp6fFaLT/zKVSvh9V1MKTVL/",
      "publicKey": {
        "content": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUMrRENDQW42Z0F3SUJBZ0lVQW5DZkFwZGZBQjJBU01sTjNlK3I5WTZyV1hvd0NnWUlLb1pJemowRUF3TXcKTnpFVk1CTUdBMVVFQ2hNTWMybG5jM1J2Y21VdVpHVjJNUjR3SEFZRFZRUURFeFZ6YVdkemRHOXlaUzFwYm5SbApjbTFsWkdsaGRHVXdIaGNOTWpNd05UQTNNVE14TlRFeVdoY05Nak13TlRBM01UTXlOVEV5V2pBQU1Ga3dFd1lICktvWkl6ajBDQVFZSUtvWkl6ajBEQVFjRFFnQUUwQnVGUzdaTlVRdFpTd0lLZEdXZXA1U0NScnY3RmVsWmYzR3gKMDN6aVFhTDNQbmFvOEJCWE11VTNNY2ZVUWRXdExHdEhxVm43bmZ0OUJwY3lRZHM5RmFPQ0FaMHdnZ0daTUE0RwpBMVVkRHdFQi93UUVBd0lIZ0RBVEJnTlZIU1VFRERBS0JnZ3JCZ0VGQlFjREF6QWRCZ05WSFE0RUZnUVVEUTVECjJqRncyN1lnT0NneFhUdWFXNldlcW9Vd0h3WURWUjBqQkJnd0ZvQVUzOVBwejFZa0VaYjVxTmpwS0ZXaXhpNFkKWkQ4d1RBWURWUjBSQVFIL0JFSXdRSUUrWTI5emFXZHVMWE4yWXkxaFkyTnZkVzUwUUcxcGJtVnlZV3d0YldsdQpkWFJwWVMwNE1qQXVhV0Z0TG1kelpYSjJhV05sWVdOamIzVnVkQzVqYjIwd0tRWUtLd1lCQkFHRHZ6QUJBUVFiCmFIUjBjSE02THk5aFkyTnZkVzUwY3k1bmIyOW5iR1V1WTI5dE1Dc0dDaXNHQVFRQmc3OHdBUWdFSFF3YmFIUjAKY0hNNkx5OWhZMk52ZFc1MGN5NW5iMjluYkdVdVkyOXRNSUdMQmdvckJnRUVBZFo1QWdRQ0JIMEVld0I1QUhjQQozVDB3YXNiSEVUSmpHUjRjbVdjM0FxSktYcmplUEszL2g0cHlnQzhwN280QUFBR0g5bG9hMkFBQUJBTUFTREJHCkFpRUE0RC9hYUdscUU3K2h4Vkc2RG9NS1l0ajBpRC9mV0htZUZFZDRNMWdVbTNzQ0lRRDE4bTBJK2t5ODliMk0KWkdpb0p1c29jVEVTODBJdDJwVDZEQ09RRHFYZU5UQUtCZ2dxaGtqT1BRUURBd05vQURCbEFqQWtTbSt5VmpWeQpWb1NCODdCR0d4RzVsalEzYVdRWlVkbDU5MmJTSmo2SmZVSTN4cXBCTklLN2sxQlNjMWhNbFBVQ01RQ2U2ai9JCm9EV2lCOW9JeVpCVXFGajJDemhRNENpS1FaYWpVTmlQa0RQbUZyODhsNzR4L2FKcVNob0J6Vkh1OG53PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
      }
    }
  }
}
```

from there the base64encoded `publicKey` is what was issued during the [signing ceremony](https://docs.sigstore.dev/fulcio/certificate-issuing-overview). 

```
-----BEGIN CERTIFICATE-----
MIIC+DCCAn6gAwIBAgIUAnCfApdfAB2ASMlN3e+r9Y6rWXowCgYIKoZIzj0EAwMw
NzEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MR4wHAYDVQQDExVzaWdzdG9yZS1pbnRl
cm1lZGlhdGUwHhcNMjMwNTA3MTMxNTEyWhcNMjMwNTA3MTMyNTEyWjAAMFkwEwYH
KoZIzj0CAQYIKoZIzj0DAQcDQgAE0BuFS7ZNUQtZSwIKdGWep5SCRrv7FelZf3Gx
03ziQaL3Pnao8BBXMuU3McfUQdWtLGtHqVn7nft9BpcyQds9FaOCAZ0wggGZMA4G
A1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAdBgNVHQ4EFgQUDQ5D
2jFw27YgOCgxXTuaW6WeqoUwHwYDVR0jBBgwFoAU39Ppz1YkEZb5qNjpKFWixi4Y
ZD8wTAYDVR0RAQH/BEIwQIE+Y29zaWduLXN2Yy1hY2NvdW50QG1pbmVyYWwtbWlu
dXRpYS04MjAuaWFtLmdzZXJ2aWNlYWNjb3VudC5jb20wKQYKKwYBBAGDvzABAQQb
aHR0cHM6Ly9hY2NvdW50cy5nb29nbGUuY29tMCsGCisGAQQBg78wAQgEHQwbaHR0
cHM6Ly9hY2NvdW50cy5nb29nbGUuY29tMIGLBgorBgEEAdZ5AgQCBH0EewB5AHcA
3T0wasbHETJjGR4cmWc3AqJKXrjePK3/h4pygC8p7o4AAAGH9loa2AAABAMASDBG
AiEA4D/aaGlqE7+hxVG6DoMKYtj0iD/fWHmeFEd4M1gUm3sCIQD18m0I+ky89b2M
ZGioJusocTES80It2pT6DCOQDqXeNTAKBggqhkjOPQQDAwNoADBlAjAkSm+yVjVy
VoSB87BGGxG5ljQ3aWQZUdl592bSJj6JfUI3xqpBNIK7k1BSc1hMlPUCMQCe6j/I
oDWiB9oIyZBUqFj2CzhQ4CiKQZajUNiPkDPmFr88l74x/aJqShoBzVHu8nw=
-----END CERTIFICATE-----
```

which expanded is 

```bash
$  openssl x509 -in cosign.crt -noout -text

Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number:
            02:70:9f:02:97:5f:00:1d:80:48:c9:4d:dd:ef:ab:f5:8e:ab:59:7a
        Signature Algorithm: ecdsa-with-SHA384
        Issuer: O = sigstore.dev, CN = sigstore-intermediate
        Validity
            Not Before: May  7 13:15:12 2023 GMT
            Not After : May  7 13:25:12 2023 GMT
        Subject: 
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
                Public-Key: (256 bit)
                pub:
                    04:d0:1b:85:4b:b6:4d:51:0b:59:4b:02:0a:74:65:
                    9e:a7:94:82:46:bb:fb:15:e9:59:7f:71:b1:d3:7c:
                    e2:41:a2:f7:3e:76:a8:f0:10:57:32:e5:37:31:c7:
                    d4:41:d5:ad:2c:6b:47:a9:59:fb:9d:fb:7d:06:97:
                    32:41:db:3d:15
                ASN1 OID: prime256v1
                NIST CURVE: P-256
        X509v3 extensions:
            X509v3 Key Usage: critical
                Digital Signature
            X509v3 Extended Key Usage: 
                Code Signing
            X509v3 Subject Key Identifier: 
                0D:0E:43:DA:31:70:DB:B6:20:38:28:31:5D:3B:9A:5B:A5:9E:AA:85
            X509v3 Authority Key Identifier: 
                DF:D3:E9:CF:56:24:11:96:F9:A8:D8:E9:28:55:A2:C6:2E:18:64:3F
            X509v3 Subject Alternative Name: critical
                email:cosign-svc-account@$PROJECT_ID.iam.gserviceaccount.com  <<<<<<<<<<<<<<<<<<<<<
            1.3.6.1.4.1.57264.1.1: 
                https://accounts.google.com
            1.3.6.1.4.1.57264.1.8: 
                ..https://accounts.google.com
            CT Precertificate SCTs: 
                Signed Certificate Timestamp:
                    Version   : v1 (0x0)
                    Log ID    : DD:3D:30:6A:C6:C7:11:32:63:19:1E:1C:99:67:37:02:
                                A2:4A:5E:B8:DE:3C:AD:FF:87:8A:72:80:2F:29:EE:8E
                    Timestamp : May  7 13:15:12.984 2023 GMT
                    Extensions: none
                    Signature : ecdsa-with-SHA256
                                30:46:02:21:00:E0:3F:DA:68:69:6A:13:BF:A1:C5:51:
                                BA:0E:83:0A:62:D8:F4:88:3F:DF:58:79:9E:14:47:78:
                                33:58:14:9B:7B:02:21:00:F5:F2:6D:08:FA:4C:BC:F5:
                                BD:8C:64:68:A8:26:EB:28:71:31:12:F3:42:2D:DA:94:
                                FA:0C:23:90:0E:A5:DE:35
    Signature Algorithm: ecdsa-with-SHA384
```

NOTE the OID `1.3.6.1.4.1.57264.1.1` is registered to [here](https://github.com/sigstore/fulcio/blob/main/docs/oid-info.md#directory) and denotes the OIDC Token's issuer

Now use `rekor-cli` to search for what we added to the transparency log using


* `sha` value from `hashedrekord`

```bash
$ rekor-cli search --rekor_server https://rekor.sigstore.dev \
   --sha  cb314ddc9783c1812afe8235fd2b7d6323e408cc53d06a69ccba604510a5ca85

Found matching entries (listed by UUID):
24296fb24b8ad77a4d23c653c2208982a20fea77430945efe71292049c3a91dbfef8a0f068954bc7
```

* the email for the build service account's `OIDC`

```bash
$ rekor-cli search --rekor_server https://rekor.sigstore.dev  --email cosign-svc-account@$PROJECT_ID.iam.gserviceaccount.com

Found matching entries (listed by UUID):
24296fb24b8ad77ad3f3239fbd7b5c54a3d2dd5bce76ff15519601d248f27b5e9e62279996d15237
24296fb24b8ad77a4d23c653c2208982a20fea77430945efe71292049c3a91dbfef8a0f068954bc7
24296fb24b8ad77a83fc9f520f734aab62f2c49955ed4fc12376e5a705c5d3b4ddb16aad1ae5bb3b
```

note each `UUID` asserts something different:  the `signature` and the others the two attestations we setup (predicates)

the json predicate:

```bash
$ rekor-cli get --rekor_server https://rekor.sigstore.dev  \
  --uuid 24296fb24b8ad77ad3f3239fbd7b5c54a3d2dd5bce76ff15519601d248f27b5e9e62279996d15237

LogID: c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d
Attestation: {"_type":"https://in-toto.io/Statement/v0.1","predicateType":"cosign.sigstore.dev/attestation/v1","subject":[{"name":"us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel","digest":{"sha256":"a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00"}}],"predicate":{"Data":"{ \"projectid\": \"$PROJECT_ID\", \"buildid\": \"e31bc37c-40df-414c-9e6a-286b85d8ca31\", \"foo\":\"bar\", \"commitsha\": \"71d4d7251cc8f993ad4094ddda33a65fee9c4df4\", \"name_hash\": \"$(cat /workspace/name_hash.txt)\"}","Timestamp":"2023-05-07T13:15:17Z"}}
Index: 19941764
IntegratedTime: 2023-05-07T13:15:17Z
UUID: 24296fb24b8ad77ad3f3239fbd7b5c54a3d2dd5bce76ff15519601d248f27b5e9e62279996d15237
Body: {
  "IntotoObj": {
    "content": {
      "hash": {
        "algorithm": "sha256",
        "value": "a741ae879dfa5b2f03d59a6d5b06f69c7c0479c1b55c18a596e1a61915cdc9ec"
      },
      "payloadHash": {
        "algorithm": "sha256",
        "value": "c3a1143ad6e7b20b617442b424ccfd86e4647b04603eb36d972e06e78506ed46"
      }
    },
    "publicKey": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUM5ekNDQW4yZ0F3SUJBZ0lVWEtLckFoOVZ1WXBtTGIyVWdwbE9iTWZnZEpRd0NnWUlLb1pJemowRUF3TXcKTnpFVk1CTUdBMVVFQ2hNTWMybG5jM1J2Y21VdVpHVjJNUjR3SEFZRFZRUURFeFZ6YVdkemRHOXlaUzFwYm5SbApjbTFsWkdsaGRHVXdIaGNOTWpNd05UQTNNVE14TlRFMldoY05Nak13TlRBM01UTXlOVEUyV2pBQU1Ga3dFd1lICktvWkl6ajBDQVFZSUtvWkl6ajBEQVFjRFFnQUVrRFZmT3BOMUkzUEJmNDVacHBmb28xVkxJeEdrVFdoMHRucWQKRVNLK1Z4aG1wQysvZXdjSnlFd3VzU0kvb28zSm9zTnBobTF2UU14aWRyTllZOHBhT2FPQ0Fad3dnZ0dZTUE0RwpBMVVkRHdFQi93UUVBd0lIZ0RBVEJnTlZIU1VFRERBS0JnZ3JCZ0VGQlFjREF6QWRCZ05WSFE0RUZnUVVtUkFFClExbUhIRjZTZkFjbi9KdWJBcjJUNERZd0h3WURWUjBqQkJnd0ZvQVUzOVBwejFZa0VaYjVxTmpwS0ZXaXhpNFkKWkQ4d1RBWURWUjBSQVFIL0JFSXdRSUUrWTI5emFXZHVMWE4yWXkxaFkyTnZkVzUwUUcxcGJtVnlZV3d0YldsdQpkWFJwWVMwNE1qQXVhV0Z0TG1kelpYSjJhV05sWVdOamIzVnVkQzVqYjIwd0tRWUtLd1lCQkFHRHZ6QUJBUVFiCmFIUjBjSE02THk5aFkyTnZkVzUwY3k1bmIyOW5iR1V1WTI5dE1Dc0dDaXNHQVFRQmc3OHdBUWdFSFF3YmFIUjAKY0hNNkx5OWhZMk52ZFc1MGN5NW5iMjluYkdVdVkyOXRNSUdLQmdvckJnRUVBZFo1QWdRQ0JId0VlZ0I0QUhZQQozVDB3YXNiSEVUSmpHUjRjbVdjM0FxSktYcmplUEszL2g0cHlnQzhwN280QUFBR0g5bG9xS2dBQUJBTUFSekJGCkFpQmhxY3JPdE9LdDJNUjdTMkFoSTNwMzdRQ0h2RGFMTVdZRUNqY3FMYzdRb2dJaEFLWmxGQ3U2amwwbXIxM2MKUGdEWnVEZGZqckhDR09PdUhsanR5QXhqMEZOSk1Bb0dDQ3FHU000OUJBTURBMmdBTUdVQ01RRFFjSzlSWis0UQpCcHFZL3BvakQxOWRreml4TjJrZGFGb1liVDBHZGppWVRaa3JpdlJpU2Rxd240dTNic1E0dXNRQ01GSmh6SHhtCnM3RVJsR0U0QkhPTlUyMVJ4bHBrL2pDcm1nSkYvSFRYcWxYajg1ckw1NnozWGRXdHJlZ0VYTXc3VGc9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
  }
}
```


```bash
$ rekor-cli get --rekor_server https://rekor.sigstore.dev    --uuid 24296fb24b8ad77a4d23c653c2208982a20fea77430945efe71292049c3a91dbfef8a0f068954bc7

LogID: c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d
Index: 19941761
IntegratedTime: 2023-05-07T13:15:14Z
UUID: 24296fb24b8ad77a4d23c653c2208982a20fea77430945efe71292049c3a91dbfef8a0f068954bc7
Body: {
  "HashedRekordObj": {
    "data": {
      "hash": {
        "algorithm": "sha256",
        "value": "cb314ddc9783c1812afe8235fd2b7d6323e408cc53d06a69ccba604510a5ca85"
      }
    },
    "signature": {
      "content": "MEYCIQCwYn+zSZPxw61HdkTodC0Uh5ksP1+c2rc78IUzZ8FcpQIhAJELPodwpRh4WdZUutaWBp6fFaLT/zKVSvh9V1MKTVL/",
      "publicKey": {
        "content": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUMrRENDQW42Z0F3SUJBZ0lVQW5DZkFwZGZBQjJBU01sTjNlK3I5WTZyV1hvd0NnWUlLb1pJemowRUF3TXcKTnpFVk1CTUdBMVVFQ2hNTWMybG5jM1J2Y21VdVpHVjJNUjR3SEFZRFZRUURFeFZ6YVdkemRHOXlaUzFwYm5SbApjbTFsWkdsaGRHVXdIaGNOTWpNd05UQTNNVE14TlRFeVdoY05Nak13TlRBM01UTXlOVEV5V2pBQU1Ga3dFd1lICktvWkl6ajBDQVFZSUtvWkl6ajBEQVFjRFFnQUUwQnVGUzdaTlVRdFpTd0lLZEdXZXA1U0NScnY3RmVsWmYzR3gKMDN6aVFhTDNQbmFvOEJCWE11VTNNY2ZVUWRXdExHdEhxVm43bmZ0OUJwY3lRZHM5RmFPQ0FaMHdnZ0daTUE0RwpBMVVkRHdFQi93UUVBd0lIZ0RBVEJnTlZIU1VFRERBS0JnZ3JCZ0VGQlFjREF6QWRCZ05WSFE0RUZnUVVEUTVECjJqRncyN1lnT0NneFhUdWFXNldlcW9Vd0h3WURWUjBqQkJnd0ZvQVUzOVBwejFZa0VaYjVxTmpwS0ZXaXhpNFkKWkQ4d1RBWURWUjBSQVFIL0JFSXdRSUUrWTI5emFXZHVMWE4yWXkxaFkyTnZkVzUwUUcxcGJtVnlZV3d0YldsdQpkWFJwWVMwNE1qQXVhV0Z0TG1kelpYSjJhV05sWVdOamIzVnVkQzVqYjIwd0tRWUtLd1lCQkFHRHZ6QUJBUVFiCmFIUjBjSE02THk5aFkyTnZkVzUwY3k1bmIyOW5iR1V1WTI5dE1Dc0dDaXNHQVFRQmc3OHdBUWdFSFF3YmFIUjAKY0hNNkx5OWhZMk52ZFc1MGN5NW5iMjluYkdVdVkyOXRNSUdMQmdvckJnRUVBZFo1QWdRQ0JIMEVld0I1QUhjQQozVDB3YXNiSEVUSmpHUjRjbVdjM0FxSktYcmplUEszL2g0cHlnQzhwN280QUFBR0g5bG9hMkFBQUJBTUFTREJHCkFpRUE0RC9hYUdscUU3K2h4Vkc2RG9NS1l0ajBpRC9mV0htZUZFZDRNMWdVbTNzQ0lRRDE4bTBJK2t5ODliMk0KWkdpb0p1c29jVEVTODBJdDJwVDZEQ09RRHFYZU5UQUtCZ2dxaGtqT1BRUURBd05vQURCbEFqQWtTbSt5VmpWeQpWb1NCODdCR0d4RzVsalEzYVdRWlVkbDU5MmJTSmo2SmZVSTN4cXBCTklLN2sxQlNjMWhNbFBVQ01RQ2U2ai9JCm9EV2lCOW9JeVpCVXFGajJDemhRNENpS1FaYWpVTmlQa0RQbUZyODhsNzR4L2FKcVNob0J6Vkh1OG53PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
      }
    }
  }
}
```

the sbom attestation
```bash
$ rekor-cli get --rekor_server https://rekor.sigstore.dev    --uuid 24296fb24b8ad77a83fc9f520f734aab62f2c49955ed4fc12376e5a705c5d3b4ddb16aad1ae5bb3b

LogID: c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d
Attestation: {"_type":"https://in-toto.io/Statement/v0.1","predicateType":"https://cyclonedx.org/bom/v1.4","subject":[{"name":"us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel","digest":{"sha256":"a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00"}}],"predicate":{"bomFormat":"CycloneDX","components":[{"bom-ref":"pkg:deb/debian/base-files@10.3+deb10u9?arch=amd64\u0026distro=debian-10\u0026package-id=5aa6e4929bf16696","cpe":"cpe:2.3:a:base-files:base-files:10.3\\+deb10u9:*:*:*:*:*:*:*","licenses":[{"license":{"name":"GPL"}}],"name":"base-files","properties":[{"name":"syft:package:foundBy","value":"dpkgdb-cataloger"},{"name":"syft:package:metadataType","value":"DpkgMetadata"},{"name":"syft:package:type","value":"deb"},{"name":"syft:cpe23","value":"cpe:2.3:a:base-files:base_files:10.3\\+deb10u9:*:*:*:*:*:*:*"},{"name":"syft:cpe23","value":"cpe:2.3:a:base_files:base-files:10.3\\+deb10u9:*:*:*:*:*:*:*"},{"name":"syft:cpe23","value":"cpe:2.3:a:base_files:base_files:10.3\\+deb10u9:*:*:*:*:*:*:*"},{"name":"syft:cpe23","value":"cpe:2.3:a:base:base-files:10.3\\+deb10u9:*:*:*:*:*:*:*"},{"name":"syft:cpe23","value":"cpe:2.3:a:base:base_files:10.3\\+deb10u9:*:*:*:*:*:*:*"},{"name":"syft:location:0:layerID","value":"sha256:417cb9b79adeec55f58b890dc9831e252e3523d8de5fd28b4ee2abb151b7dc8b"},{"name":"syft:location:0:path","value":"/usr/share/doc/base-files/copyright"},{"name":"syft:location:1:layerID","value":"sha256:417cb9b79adeec55f58b890dc9831e252e3523d8de5fd28b4ee2abb151b7dc8b"},{"name":"syft:location:1:path","value":"/var/lib/dpkg/status.d/base"},{"name":"syft:metadata:installedSize","value":"340"}],"publisher":"Santiago Vila \u003csanvila@debian.org\u003e","purl":"pkg:deb/debian/base-files@10.3+deb10u9?arch=amd64\u0026distro=debian-10","type":"library","version":"10.3+deb10u9"},{"bom-ref":"pkg:deb/debian/libc6@2.28-10?arch=amd64\u0026upstream=glibc\u0026distro=debian-10\u0026package-id=74ac5ee7adfb6a2d","cpe":"cpe:2.3:a:libc6:libc6:2.28-10:*:*:*:*:*:*:*","licenses":[{"license":{"id":"GPL-2.0-only"}},{"license":{"id":"LGPL-2.1-only"}}],"name":"libc6","properties":[{"name":"syft:package:foundBy","value":"dpkgdb-cataloger"},{"name":"syft:package:metadataType","value":"DpkgMetadata"},{"name":"syft:package:type","value":"deb"},{"name":"syft:location:0:layerID","value":"sha256:5d09c2db1d761a6cd292a453815df5fecfa98058e4f2c0185976d1f28dfc09ec"},{"name":"syft:location:0:path","value":"/usr/share/doc/libc6/copyright"},{"name":"syft:location:1:layerID","value":"sha256:5d09c2db1d761a6cd292a453815df5fecfa98058e4f2c0185976d1f28dfc09ec"},{"name":"syft:location:1:path","value":"/var/lib/dpkg/status.d/libc6"},{"name":"syft:metadata:installedSize","value":"12337"},{"name":"syft:metadata:source","value":"glibc"}],"publisher":"GNU Libc Maintainers \u003cdebian-glibc@lists.debian.org\u003e","purl":"pkg:deb/debian/libc6@2.28-10?arch=amd64\u0026upstream=glibc\u0026distro=debian-10","type":"library","version":"2.28-10"},{"bom-ref":"pkg:deb/debian/libssl1.1@1.1.1d-0+deb10u6?arch=amd64\u0026upstream=openssl\u0026distro=debian-10\u0026package-id=ab8b40f4f3d74be0","cpe":"cpe:2.3:a:libssl1.1:libssl1.1:1.1.1d-0\\+deb10u6:*:*:*:*:*:*:*","name":"libssl1.1","properties":[{"name":"syft:package:foundBy","value":"dpkgdb-cataloger"},{"name":"syft:package:metadataType","value":"DpkgMetadata"},{"name":"syft:package:type","value":"deb"},{"name":"syft:location:0:layerID","value":"sha256:5d09c2db1d761a6cd292a453815df5fecfa98058e4f2c0185976d1f28dfc09ec"},{"name":"syft:location:0:path","value":"/usr/share/doc/libssl1.1/copyright"},{"name":"syft:location:1:layerID","value":"sha256:5d09c2db1d761a6cd292a453815df5fecfa98058e4f2c0185976d1f28dfc09ec"},{"name":"syft:location:1:path","value":"/var/lib/dpkg/status.d/libssl1"},{"name":"syft:metadata:installedSize","value":"4077"},{"name":"syft:metadata:source","value":"openssl"}],"publisher":"Debian OpenSSL Team \u003cpkg-openssl-devel@lists.alioth.debian.org\u003e","purl":"pkg:deb/debian/libssl1.1@1.1.1d-0+deb10u6?arch=amd64\u0026upstream=openssl\u0026distro=debian-10","type":"library","version":"1.1.1d-0+deb10u6"},{"bom-ref":"pkg:deb/debian/netbase@5.6?arch=all\u0026distro=debian-10\u0026package-id=b55e51dca4eba9a6","cpe":"cpe:2.3:a:netbase:netbase:5.6:*:*:*:*:*:*:*","licenses":[{"license":{"id":"GPL-2.0-only"}}],"name":"netbase","properties":[{"name":"syft:package:foundBy","value":"dpkgdb-cataloger"},{"name":"syft:package:metadataType","value":"DpkgMetadata"},{"name":"syft:package:type","value":"deb"},{"name":"syft:location:0:layerID","value":"sha256:417cb9b79adeec55f58b890dc9831e252e3523d8de5fd28b4ee2abb151b7dc8b"},{"name":"syft:location:0:path","value":"/usr/share/doc/netbase/copyright"},{"name":"syft:location:1:layerID","value":"sha256:417cb9b79adeec55f58b890dc9831e252e3523d8de5fd28b4ee2abb151b7dc8b"},{"name":"syft:location:1:path","value":"/var/lib/dpkg/status.d/netbase"},{"name":"syft:metadata:installedSize","value":"44"}],"publisher":"Marco d'Itri \u003cmd@linux.it\u003e","purl":"pkg:deb/debian/netbase@5.6?arch=all\u0026distro=debian-10","type":"library","version":"5.6"},{"bom-ref":"pkg:deb/debian/openssl@1.1.1d-0+deb10u6?arch=amd64\u0026distro=debian-10\u0026package-id=5baa662d4c747c2e","cpe":"cpe:2.3:a:openssl:openssl:1.1.1d-0\\+deb10u6:*:*:*:*:*:*:*","name":"openssl","properties":[{"name":"syft:package:foundBy","value":"dpkgdb-cataloger"},{"name":"syft:package:metadataType","value":"DpkgMetadata"},{"name":"syft:package:type","value":"deb"},{"name":"syft:location:0:layerID","value":"sha256:5d09c2db1d761a6cd292a453815df5fecfa98058e4f2c0185976d1f28dfc09ec"},{"name":"syft:location:0:path","value":"/usr/share/doc/openssl/copyright"},{"name":"syft:location:1:layerID","value":"sha256:5d09c2db1d761a6cd292a453815df5fecfa98058e4f2c0185976d1f28dfc09ec"},{"name":"syft:location:1:path","value":"/var/lib/dpkg/status.d/openssl"},{"name":"syft:metadata:installedSize","value":"1460"}],"publisher":"Debian OpenSSL Team \u003cpkg-openssl-devel@lists.alioth.debian.org\u003e","purl":"pkg:deb/debian/openssl@1.1.1d-0+deb10u6?arch=amd64\u0026distro=debian-10","type":"library","version":"1.1.1d-0+deb10u6"},{"bom-ref":"pkg:deb/debian/tzdata@2021a-0+deb10u1?arch=all\u0026distro=debian-10\u0026package-id=9e5b2198bbbd7fb0","cpe":"cpe:2.3:a:tzdata:tzdata:2021a-0\\+deb10u1:*:*:*:*:*:*:*","name":"tzdata","properties":[{"name":"syft:package:foundBy","value":"dpkgdb-cataloger"},{"name":"syft:package:metadataType","value":"DpkgMetadata"},{"name":"syft:package:type","value":"deb"},{"name":"syft:location:0:layerID","value":"sha256:417cb9b79adeec55f58b890dc9831e252e3523d8de5fd28b4ee2abb151b7dc8b"},{"name":"syft:location:0:path","value":"/usr/share/doc/tzdata/copyright"},{"name":"syft:location:1:layerID","value":"sha256:417cb9b79adeec55f58b890dc9831e252e3523d8de5fd28b4ee2abb151b7dc8b"},{"name":"syft:location:1:path","value":"/var/lib/dpkg/status.d/tzdata"},{"name":"syft:metadata:installedSize","value":"3040"}],"publisher":"GNU Libc Maintainers \u003cdebian-glibc@lists.debian.org\u003e","purl":"pkg:deb/debian/tzdata@2021a-0+deb10u1?arch=all\u0026distro=debian-10","type":"library","version":"2021a-0+deb10u1"},{"description":"Distroless","externalReferences":[{"type":"issue-tracker","url":"https://github.com/GoogleContainerTools/distroless/issues/new"},{"type":"website","url":"https://github.com/GoogleContainerTools/distroless"},{"comment":"support","type":"other","url":"https://github.com/GoogleContainerTools/distroless/blob/master/README.md"}],"name":"debian","properties":[{"name":"syft:distro:id","value":"debian"},{"name":"syft:distro:prettyName","value":"Distroless"},{"name":"syft:distro:versionID","value":"10"}],"swid":{"name":"debian","tagId":"debian","version":"10"},"type":"operating-system","version":"10"}],"metadata":{"component":{"bom-ref":"3e1aa3abe8f8e099","name":"us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00","type":"container","version":"sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00"},"timestamp":"2023-05-07T13:15:08Z","tools":[{"name":"syft","vendor":"anchore","version":"0.78.0"}]},"serialNumber":"urn:uuid:62af58c9-e122-4770-bb8f-c2f7b9d4a6ca","specVersion":"1.4","version":1}}
Index: 19941758
IntegratedTime: 2023-05-07T13:15:10Z
UUID: 24296fb24b8ad77a83fc9f520f734aab62f2c49955ed4fc12376e5a705c5d3b4ddb16aad1ae5bb3b
Body: {
  "IntotoObj": {
    "content": {
      "hash": {
        "algorithm": "sha256",
        "value": "cca71b8b5fd32562a40e99de1926d7a7b0005c52086dcd78ce2e8cb2d4445571"
      },
      "payloadHash": {
        "algorithm": "sha256",
        "value": "8979c7e6aa26d69a78b49464e8f23d821524f2c961c15a6eae09b496570a91ff"
      }
    },
    "publicKey": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUMrRENDQW42Z0F3SUJBZ0lVYjZLNGlacGx5NTdsODZoS0FITVVrL1k0V3FFd0NnWUlLb1pJemowRUF3TXcKTnpFVk1CTUdBMVVFQ2hNTWMybG5jM1J2Y21VdVpHVjJNUjR3SEFZRFZRUURFeFZ6YVdkemRHOXlaUzFwYm5SbApjbTFsWkdsaGRHVXdIaGNOTWpNd05UQTNNVE14TlRBNVdoY05Nak13TlRBM01UTXlOVEE1V2pBQU1Ga3dFd1lICktvWkl6ajBDQVFZSUtvWkl6ajBEQVFjRFFnQUV0TUNJYzlmc1cvOXZtWmg1MFlCRlhjUjZuN3FEWjU3UGR2L2UKSnlaVXVZRGJMMU9BTW0ydTJJOWVZaUs2Y1pMaWdtWDhhc0ltYk9WaGxWTkJSOGdzN2FPQ0FaMHdnZ0daTUE0RwpBMVVkRHdFQi93UUVBd0lIZ0RBVEJnTlZIU1VFRERBS0JnZ3JCZ0VGQlFjREF6QWRCZ05WSFE0RUZnUVVkWFNjCi8rVkZCcTBOU05ib3AwQjBLK1g4RVlvd0h3WURWUjBqQkJnd0ZvQVUzOVBwejFZa0VaYjVxTmpwS0ZXaXhpNFkKWkQ4d1RBWURWUjBSQVFIL0JFSXdRSUUrWTI5emFXZHVMWE4yWXkxaFkyTnZkVzUwUUcxcGJtVnlZV3d0YldsdQpkWFJwWVMwNE1qQXVhV0Z0TG1kelpYSjJhV05sWVdOamIzVnVkQzVqYjIwd0tRWUtLd1lCQkFHRHZ6QUJBUVFiCmFIUjBjSE02THk5aFkyTnZkVzUwY3k1bmIyOW5iR1V1WTI5dE1Dc0dDaXNHQVFRQmc3OHdBUWdFSFF3YmFIUjAKY0hNNkx5OWhZMk52ZFc1MGN5NW5iMjluYkdVdVkyOXRNSUdMQmdvckJnRUVBZFo1QWdRQ0JIMEVld0I1QUhjQQozVDB3YXNiSEVUSmpHUjRjbVdjM0FxSktYcmplUEszL2g0cHlnQzhwN280QUFBR0g5bG9QT3dBQUJBTUFTREJHCkFpRUFxOTk4VzFDL1FienhWYnJVSzFzdklJV1RCc01JRXRGa0hNanhxdlNCT2R3Q0lRQ0dOSGh0WXdwUUo3cVgKMUh1MGcwdnVYeVE1eDdsdjlmL056Y1RKa3ZsN01EQUtCZ2dxaGtqT1BRUURBd05vQURCbEFqQStRdi9LMEU0VgpFNjlYWXB1anhLTHc4TWhNaEY0amxNcUF3RDR3enpERWVGS0o2Mmo0Mm9SMldqSlN2MUt1K3RVQ01RQ08vaERHClowdDFYZXpxZ1UzdjY2SkQ5cnJmbE1sYkhGaEFkT0VBOVAzLzJWVkpPRUVVSUt3RG9FbnhzY3h2TXBZPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
  }
}
```


(just note the timestamps on the two certs are like 2s apart (which was about when the build steps happened for each step;  also note the email SAN is the sam))


### Crane

You can also view the registry manifest for the signature using [crane](github.com/google/go-containerregistry/cmd/crane)

You'll see the two signatures (one for KMS, another larger one for the OIDC signature metadata)

```text
# go install github.com/google/go-containerregistry/cmd/crane@latest

$ crane  manifest us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel:sha256-a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00.sig | jq '.'


{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": {
    "mediaType": "application/vnd.oci.image.config.v1+json",
    "size": 352,
    "digest": "sha256:e778177db0900b5f989f9b66ffd5df9629840db59da5e4329ef1d2eb32d30879"
  },
  "layers": [
    {
      "mediaType": "application/vnd.dev.cosign.simplesigning.v1+json",
      "size": 299,
      "digest": "sha256:cb314ddc9783c1812afe8235fd2b7d6323e408cc53d06a69ccba604510a5ca85",
      "annotations": {
        "dev.cosignproject.cosign/signature": "MEQCIQCmLVfr2RjHE/U7oZ8EcuGZciDxUKVZZR5mSgjD7verUgIfbwscbnxHV9EDdzPkvv6olTWKQxkJaKS9tlwBwgOp5Q=="
      }
    },
    {
      "mediaType": "application/vnd.dev.cosign.simplesigning.v1+json",
      "size": 299,
      "digest": "sha256:cb314ddc9783c1812afe8235fd2b7d6323e408cc53d06a69ccba604510a5ca85",
      "annotations": {
        "dev.cosignproject.cosign/signature": "MEYCIQCwYn+zSZPxw61HdkTodC0Uh5ksP1+c2rc78IUzZ8FcpQIhAJELPodwpRh4WdZUutaWBp6fFaLT/zKVSvh9V1MKTVL/",
        "dev.sigstore.cosign/bundle": "{\"SignedEntryTimestamp\":\"MEUCIF0BDv4ma9gi2t7P15xlAnmRAzIqDGuDjc+myBQugfuVAiEA+fDQZRSwHvOPxrRGPxxah3l28eaEUIe6SqMzVw/qXtY=\",\"Payload\":{\"body\":\"eyJhcGlWZXJzaW9uIjoiMC4wLjEiLCJraW5kIjoiaGFzaGVkcmVrb3JkIiwic3BlYyI6eyJkYXRhIjp7Imhhc2giOnsiYWxnb3JpdGhtIjoic2hhMjU2IiwidmFsdWUiOiJjYjMxNGRkYzk3ODNjMTgxMmFmZTgyMzVmZDJiN2Q2MzIzZTQwOGNjNTNkMDZhNjljY2JhNjA0NTEwYTVjYTg1In19LCJzaWduYXR1cmUiOnsiY29udGVudCI6Ik1FWUNJUUN3WW4relNaUHh3NjFIZGtUb2RDMFVoNWtzUDErYzJyYzc4SVV6WjhGY3BRSWhBSkVMUG9kd3BSaDRXZFpVdXRhV0JwNmZGYUxUL3pLVlN2aDlWMU1LVFZMLyIsInB1YmxpY0tleSI6eyJjb250ZW50IjoiTFMwdExTMUNSVWRKVGlCRFJWSlVTVVpKUTBGVVJTMHRMUzB0Q2sxSlNVTXJSRU5EUVc0MlowRjNTVUpCWjBsVlFXNURaa0Z3WkdaQlFqSkJVMDFzVGpObEszSTVXVFp5VjFodmQwTm5XVWxMYjFwSmVtb3dSVUYzVFhjS1RucEZWazFDVFVkQk1WVkZRMmhOVFdNeWJHNWpNMUoyWTIxVmRWcEhWakpOVWpSM1NFRlpSRlpSVVVSRmVGWjZZVmRrZW1SSE9YbGFVekZ3WW01U2JBcGpiVEZzV2tkc2FHUkhWWGRJYUdOT1RXcE5kMDVVUVROTlZFMTRUbFJGZVZkb1kwNU5hazEzVGxSQk0wMVVUWGxPVkVWNVYycEJRVTFHYTNkRmQxbElDa3R2V2tsNmFqQkRRVkZaU1V0dldrbDZhakJFUVZGalJGRm5RVVV3UW5WR1V6ZGFUbFZSZEZwVGQwbExaRWRYWlhBMVUwTlNjblkzUm1Wc1dtWXpSM2dLTURONmFWRmhURE5RYm1Gdk9FSkNXRTExVlROTlkyWlZVV1JYZEV4SGRFaHhWbTQzYm1aME9VSndZM2xSWkhNNVJtRlBRMEZhTUhkblowZGFUVUUwUndwQk1WVmtSSGRGUWk5M1VVVkJkMGxJWjBSQlZFSm5UbFpJVTFWRlJFUkJTMEpuWjNKQ1owVkdRbEZqUkVGNlFXUkNaMDVXU0ZFMFJVWm5VVlZFVVRWRUNqSnFSbmN5TjFsblQwTm5lRmhVZFdGWE5sZGxjVzlWZDBoM1dVUldVakJxUWtKbmQwWnZRVlV6T1ZCd2VqRlphMFZhWWpWeFRtcHdTMFpYYVhocE5Ga0tXa1E0ZDFSQldVUldVakJTUVZGSUwwSkZTWGRSU1VVcldUSTVlbUZYWkhWTVdFNHlXWGt4YUZreVRuWmtWelV3VVVjeGNHSnRWbmxaVjNkMFlsZHNkUXBrV0ZKd1dWTXdORTFxUVhWaFYwWjBURzFrZWxwWVNqSmhWMDVzV1ZkT2FtSXpWblZrUXpWcVlqSXdkMHRSV1V0TGQxbENRa0ZIUkhaNlFVSkJVVkZpQ21GSVVqQmpTRTAyVEhrNWFGa3lUblprVnpVd1kzazFibUl5T1c1aVIxVjFXVEk1ZEUxRGMwZERhWE5IUVZGUlFtYzNPSGRCVVdkRlNGRjNZbUZJVWpBS1kwaE5Oa3g1T1doWk1rNTJaRmMxTUdONU5XNWlNamx1WWtkVmRWa3lPWFJOU1VkTVFtZHZja0puUlVWQlpGbzFRV2RSUTBKSU1FVmxkMEkxUVVoalFRb3pWREIzWVhOaVNFVlVTbXBIVWpSamJWZGpNMEZ4U2t0WWNtcGxVRXN6TDJnMGNIbG5Remh3TjI4MFFVRkJSMGc1Ykc5aE1rRkJRVUpCVFVGVFJFSkhDa0ZwUlVFMFJDOWhZVWRzY1VVM0syaDRWa2MyUkc5TlMxbDBhakJwUkM5bVYwaHRaVVpGWkRSTk1XZFZiVE56UTBsUlJERTRiVEJKSzJ0NU9EbGlNazBLV2tkcGIwcDFjMjlqVkVWVE9EQkpkREp3VkRaRVEwOVJSSEZZWlU1VVFVdENaMmR4YUd0cVQxQlJVVVJCZDA1dlFVUkNiRUZxUVd0VGJTdDVWbXBXZVFwV2IxTkNPRGRDUjBkNFJ6VnNhbEV6WVZkUldsVmtiRFU1TW1KVFNtbzJTbVpWU1RONGNYQkNUa2xMTjJzeFFsTmpNV2hOYkZCVlEwMVJRMlUyYWk5SkNtOUVWMmxDT1c5SmVWcENWWEZHYWpKRGVtaFJORU5wUzFGYVlXcFZUbWxRYTBSUWJVWnlPRGhzTnpSNEwyRktjVk5vYjBKNlZraDFPRzUzUFFvdExTMHRMVVZPUkNCRFJWSlVTVVpKUTBGVVJTMHRMUzB0Q2c9PSJ9fX19\",\"integratedTime\":1683465314,\"logIndex\":19941761,\"logID\":\"c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d\"}}",
        "dev.sigstore.cosign/certificate": "-----BEGIN CERTIFICATE-----\nMIIC+DCCAn6gAwIBAgIUAnCfApdfAB2ASMlN3e+r9Y6rWXowCgYIKoZIzj0EAwMw\nNzEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MR4wHAYDVQQDExVzaWdzdG9yZS1pbnRl\ncm1lZGlhdGUwHhcNMjMwNTA3MTMxNTEyWhcNMjMwNTA3MTMyNTEyWjAAMFkwEwYH\nKoZIzj0CAQYIKoZIzj0DAQcDQgAE0BuFS7ZNUQtZSwIKdGWep5SCRrv7FelZf3Gx\n03ziQaL3Pnao8BBXMuU3McfUQdWtLGtHqVn7nft9BpcyQds9FaOCAZ0wggGZMA4G\nA1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAdBgNVHQ4EFgQUDQ5D\n2jFw27YgOCgxXTuaW6WeqoUwHwYDVR0jBBgwFoAU39Ppz1YkEZb5qNjpKFWixi4Y\nZD8wTAYDVR0RAQH/BEIwQIE+Y29zaWduLXN2Yy1hY2NvdW50QG1pbmVyYWwtbWlu\ndXRpYS04MjAuaWFtLmdzZXJ2aWNlYWNjb3VudC5jb20wKQYKKwYBBAGDvzABAQQb\naHR0cHM6Ly9hY2NvdW50cy5nb29nbGUuY29tMCsGCisGAQQBg78wAQgEHQwbaHR0\ncHM6Ly9hY2NvdW50cy5nb29nbGUuY29tMIGLBgorBgEEAdZ5AgQCBH0EewB5AHcA\n3T0wasbHETJjGR4cmWc3AqJKXrjePK3/h4pygC8p7o4AAAGH9loa2AAABAMASDBG\nAiEA4D/aaGlqE7+hxVG6DoMKYtj0iD/fWHmeFEd4M1gUm3sCIQD18m0I+ky89b2M\nZGioJusocTES80It2pT6DCOQDqXeNTAKBggqhkjOPQQDAwNoADBlAjAkSm+yVjVy\nVoSB87BGGxG5ljQ3aWQZUdl592bSJj6JfUI3xqpBNIK7k1BSc1hMlPUCMQCe6j/I\noDWiB9oIyZBUqFj2CzhQ4CiKQZajUNiPkDPmFr88l74x/aJqShoBzVHu8nw=\n-----END CERTIFICATE-----\n",
        "dev.sigstore.cosign/chain": "-----BEGIN CERTIFICATE-----\nMIICGjCCAaGgAwIBAgIUALnViVfnU0brJasmRkHrn/UnfaQwCgYIKoZIzj0EAwMw\nKjEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MREwDwYDVQQDEwhzaWdzdG9yZTAeFw0y\nMjA0MTMyMDA2MTVaFw0zMTEwMDUxMzU2NThaMDcxFTATBgNVBAoTDHNpZ3N0b3Jl\nLmRldjEeMBwGA1UEAxMVc2lnc3RvcmUtaW50ZXJtZWRpYXRlMHYwEAYHKoZIzj0C\nAQYFK4EEACIDYgAE8RVS/ysH+NOvuDZyPIZtilgUF9NlarYpAd9HP1vBBH1U5CV7\n7LSS7s0ZiH4nE7Hv7ptS6LvvR/STk798LVgMzLlJ4HeIfF3tHSaexLcYpSASr1kS\n0N/RgBJz/9jWCiXno3sweTAOBgNVHQ8BAf8EBAMCAQYwEwYDVR0lBAwwCgYIKwYB\nBQUHAwMwEgYDVR0TAQH/BAgwBgEB/wIBADAdBgNVHQ4EFgQU39Ppz1YkEZb5qNjp\nKFWixi4YZD8wHwYDVR0jBBgwFoAUWMAeX5FFpWapesyQoZMi0CrFxfowCgYIKoZI\nzj0EAwMDZwAwZAIwPCsQK4DYiZYDPIaDi5HFKnfxXx6ASSVmERfsynYBiX2X6SJR\nnZU84/9DZdnFvvxmAjBOt6QpBlc4J/0DxvkTCqpclvziL6BCCPnjdlIB3Pu3BxsP\nmygUY7Ii2zbdCdliiow=\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\nMIIB9zCCAXygAwIBAgIUALZNAPFdxHPwjeDloDwyYChAO/4wCgYIKoZIzj0EAwMw\nKjEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MREwDwYDVQQDEwhzaWdzdG9yZTAeFw0y\nMTEwMDcxMzU2NTlaFw0zMTEwMDUxMzU2NThaMCoxFTATBgNVBAoTDHNpZ3N0b3Jl\nLmRldjERMA8GA1UEAxMIc2lnc3RvcmUwdjAQBgcqhkjOPQIBBgUrgQQAIgNiAAT7\nXeFT4rb3PQGwS4IajtLk3/OlnpgangaBclYpsYBr5i+4ynB07ceb3LP0OIOZdxex\nX69c5iVuyJRQ+Hz05yi+UF3uBWAlHpiS5sh0+H2GHE7SXrk1EC5m1Tr19L9gg92j\nYzBhMA4GA1UdDwEB/wQEAwIBBjAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBRY\nwB5fkUWlZql6zJChkyLQKsXF+jAfBgNVHSMEGDAWgBRYwB5fkUWlZql6zJChkyLQ\nKsXF+jAKBggqhkjOPQQDAwNpADBmAjEAj1nHeXZp+13NWBNa+EDsDP8G1WWg1tCM\nWP/WHPqpaVo0jhsweNFZgSs0eE7wYI4qAjEA2WB9ot98sIkoF3vZYdd3/VtWB5b9\nTNMea7Ix/stJ5TfcLLeABLE4BNJOsQ4vnBHJ\n-----END CERTIFICATE-----"
      }
    }
  ]
}
```

![images/registry.png](images/registry.png)


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
#     us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00

cosign verify-attestation \
  --key gcpkms://projects/$PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 \
    --policy policy.rego    \
      us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00  | jq '.'

will be validating against Rego policies: [policy.rego]

Verification for us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00 --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - The signatures were verified against the specified public key
{
  "payloadType": "application/vnd.in-toto+json",
  "payload": "eyJfdHlwZSI6Imh0dHBzOi8vaW4tdG90by5pby9TdGF0ZW1lbnQvdjAuMSIsInByZWRpY2F0ZVR5cGUiOiJjb3NpZ24uc2lnc3RvcmUuZGV2L2F0dGVzdGF0aW9uL3YxIiwic3ViamVjdCI6W3sibmFtZSI6InVzLWNlbnRyYWwxLWRvY2tlci5wa2cuZGV2L21pbmVyYWwtbWludXRpYS04MjAvcmVwbzEvc2VjdXJlYnVpbGQtYmF6ZWwiLCJkaWdlc3QiOnsic2hhMjU2IjoiYTJiMTA5ZmI5YmFlYTU1NTU1NjU2MTMxN2ZkZDEzY2VmOWMzZGZhYzIyYzhmOGZlYTBjNWEwYjA2ZWNlOWQwMCJ9fV0sInByZWRpY2F0ZSI6eyJEYXRhIjoieyBcInByb2plY3RpZFwiOiBcIm1pbmVyYWwtbWludXRpYS04MjBcIiwgXCJidWlsZGlkXCI6IFwiODEzY2JmMGQtMmU3Zi00OTU4LWJiYmYtNmYwODk0NzlmY2QxXCIsIFwiZm9vXCI6XCJiYXJcIiwgXCJjb21taXRzaGFcIjogXCJcIn0iLCJUaW1lc3RhbXAiOiIyMDIzLTA1LTA3VDE0OjI1OjExWiJ9fQ==",
  "signatures": [
    {
      "keyid": "",
      "sig": "MEUCIE1rtqfLKIN7iQ9d2LPyI4A6VulXuMb9j3T2NPNYLgjUAiEAjFbwOe1EYZ/yOFm3eOafEvPHgTZcgT/0LWpcoqMJOZk="
    }
  ]
}      
```

for the OIDC based signature,

```bash
COSIGN_EXPERIMENTAL=1 cosign verify-attestation  --policy policy.rego    \
        us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00  | jq '.'


Verification for us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00 --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - Existence of the claims in the transparency log was verified offline
  - Any certificates were verified against the Fulcio roots.
Certificate subject:  cosign-svc-account@$PROJECT_ID.iam.gserviceaccount.com
Certificate issuer URL:  https://accounts.google.com
{
  "payloadType": "application/vnd.in-toto+json",
  "payload": "eyJfdHlwZSI6Imh0dHBzOi8vaW4tdG90by5pby9TdGF0ZW1lbnQvdjAuMSIsInByZWRpY2F0ZVR5cGUiOiJjb3NpZ24uc2lnc3RvcmUuZGV2L2F0dGVzdGF0aW9uL3YxIiwic3ViamVjdCI6W3sibmFtZSI6InVzLWNlbnRyYWwxLWRvY2tlci5wa2cuZGV2L21pbmVyYWwtbWludXRpYS04MjAvcmVwbzEvc2VjdXJlYnVpbGQtYmF6ZWwiLCJkaWdlc3QiOnsic2hhMjU2IjoiYTJiMTA5ZmI5YmFlYTU1NTU1NjU2MTMxN2ZkZDEzY2VmOWMzZGZhYzIyYzhmOGZlYTBjNWEwYjA2ZWNlOWQwMCJ9fV0sInByZWRpY2F0ZSI6eyJEYXRhIjoieyBcInByb2plY3RpZFwiOiBcIm1pbmVyYWwtbWludXRpYS04MjBcIiwgXCJidWlsZGlkXCI6IFwiZTMxYmMzN2MtNDBkZi00MTRjLTllNmEtMjg2Yjg1ZDhjYTMxXCIsIFwiZm9vXCI6XCJiYXJcIiwgXCJjb21taXRzaGFcIjogXCI3MWQ0ZDcyNTFjYzhmOTkzYWQ0MDk0ZGRkYTMzYTY1ZmVlOWM0ZGY0XCIsIFwibmFtZV9oYXNoXCI6IFwiJChjYXQgL3dvcmtzcGFjZS9uYW1lX2hhc2gudHh0KVwifSIsIlRpbWVzdGFtcCI6IjIwMjMtMDUtMDdUMTM6MTU6MTdaIn19",
  "signatures": [
    {
      "keyid": "",
      "sig": "MEYCIQCn8jC9Ib8z10DIT4cnwAa+y6qyMEkhbHh1S0CQUFSy2AIhAIw/dDb/MrpxCJ/wTTQGAxx7cPREj5vV8j4UiXcZE/Vt"
    }
  ]
}
```

### Cosign and Rekor APIs

You can use the cosign and Rekor APIs as well.

The `client/main.go` sample application iterates over the signatures and attestations for the image hash in this repo.  


By default, it will scan the dockerhub registry but you can alter it to use your GCP Artifiact Registry.  


```bash
cd client/
go run main.go 
```


### Sign without upload to registry


The following will sign an image with a key and verify with the signatuere provided inline (`--signature`)

```bash
export IMAGE=docker.io/salrashid123/securebuild-bazel:server
export IMAGE_DIGEST=$IMAGE@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00

gcloud kms keys versions get-public-key 1  \
   --key=key1 --keyring=cosignkr  \
    --location=global --output-file=/tmp/kms_pub.pem


### sign
$ cosign sign \
  --key gcpkms://projects/$PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 \
  --upload=false $IMAGE_DIGEST --no-tlog-upload=true --output-signature sig.txt

$ cat sig.txt 
MEUCIQC8DxvtD88nqrJxQjKjfQRb3zPpT1JPBsDvQGKVdTl/zAIgTD1uPeV7XAXqQ/NDLEAWJmv6pkocyv3e4KAoc9I/HqY=

cosign verify  --key /tmp/kms_pub.pem $IMAGE_DIGEST --signature sig.txt | jq '.'

## if you want it added to the registry, the *owner* of that registry must attach the signature
cosign attach signature --signature `cat sig.txt` $IMAGE_DIGEST

### attest

cosign attest \
  --key gcpkms://projects/$PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 \
  -f --no-tlog-upload=true --no-upload=true \
  --predicate=predicate.json  $IMAGE_DIGEST --output-file=/tmp/attest.txt


cat /tmp/attest.txt  | jq '.'
```

### Sign offline and attach

The following will allow two different signer to create signatures using their own keys and then the repo owner can `attach` the signatures to the registry.  

```bash
export IMAGE=us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel:server
export IMAGE_DIGEST=us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00

### Create a signer
cosign generate-key-pair

mv cosign.key c1.key
mv cosign.pub c1.pub

cosign sign \
  --key c1.key \
  --upload=false $IMAGE_DIGEST --no-tlog-upload=true --output-signature sig.txt

cosign verify  --key c1.pub $IMAGE_DIGEST --signature sig.txt | jq '.'

$ cosign tree us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00

# attach as repo owner
cosign attach signature --signature `cat sig.txt` $IMAGE_DIGEST

$ cosign tree us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00


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
```


### Verify the image `sbom`

```bash
cosign verify --key kms_pub.pem --attachment=sbom \
         us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel@sha256:a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00  | jq '.'

Verification for us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel:sha256-a2b109fb9baea555556561317fdd13cef9c3dfac22c8f8fea0c5a0b06ece9d00.sbom --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - The signatures were verified against the specified public key
[
  {
    "critical": {
      "identity": {
        "docker-reference": "us-central1-docker.pkg.dev/$PROJECT_ID/repo1/securebuild-bazel"
      },
      "image": {
        "docker-manifest-digest": "sha256:8b2d7bc63994ae263d2058cd4237ced05f007f97a8fc7725c3ffec04b237b619"
      },
      "type": "cosign container image signature"
    },
    "optional": {
      "commit_sha": "71d4d7251cc8f993ad4094ddda33a65fee9c4df4"
    }
  }
]
```

---

