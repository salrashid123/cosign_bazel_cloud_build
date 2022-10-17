# Deterministic container hashes and container signing using Cosign, Bazel and Google Cloud Build

A simple tutorial that generates consistent container image hashes using `bazel` and then signs provenance records using [cosign](https://github.com/sigstore/cosign) (Container Signing, Verification and Storage in an OCI registry).

In this tutorial, we will:

1. generate a deterministic container image hash using  `bazel`
2. use `cosign` to create provenance records for this image
3. verify attestations and signatures using `KMS` and `OIDC` cross checked with a public transparency log.

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

* `myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753`

![images/build.png](images/build.png)

#### Push image to registry

This pushes the image to [google artifact registry](https://cloud.google.com/artifact-registry).  This will give the image hash we'd expect

You can optionally push to dockerhub if you want using [KMS based secrets](https://cloud.google.com/build/docs/securing-builds/use-encrypted-credentials#configuring_builds_to_use_encrypted_data) in cloud build 

![images/push.png](images/push.png)


#### Create attestations attributes

This step will issue a statement that includes attestation attributes users can inject into the pipeline the verifier can use. See [Verify Attestations](https://docs.sigstore.dev/cosign/verify/).

In this case, the attestation verification predicate includes some info from the build like the buildID and even the repo commithash.

Someone who wants to verify any [in-toto attestation](https://docs.sigstore.dev/cosign/attestation/) can use these values. THis repo just adds some basic stuff like the `projectID`,  `buildID` and `commitsha` (in our case, its `dd93e9e893ffaf2c4cafeb2e534bf03f66d7bf28`):


```json
{
  "_type": "https://in-toto.io/Statement/v0.1",
  "predicateType": "cosign.sigstore.dev/attestation/v1",
  "subject": [
    {
      "name": "us-central1-docker.pkg.dev/cosign-test-365209/repo1/myimage",
      "digest": {
        "sha256": "83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753"
      }
    }
  ],
  "predicate": {
    "Data": "{ 
      \"projectid\": \"cosign-test-365209\", 
      \"buildid\": \"0594f2e5-f600-48db-8aa2-daf58ee1acb5\", 
      \"foo\":\"bar\", 
      \"commitsha\": \"dd93e9e893ffaf2c4cafeb2e534bf03f66d7bf28\" }",
    "Timestamp": "2022-10-11T09:52:25Z"
  }
}
```

![images/attestations.png](images/attestations.png)

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
# for repository = "us-central1-docker.pkg.dev/$PROJECT_ID/repo1/myimage"

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
   us-central1-docker.pkg.dev/$PROJECT_ID/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753  | jq '.'

# or by api
cosign verify --key gcpkms://projects/$PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 \
      us-central1-docker.pkg.dev/$PROJECT_ID/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753 | jq '.'
```

Note this gives 

```text
Verification for us-central1-docker.pkg.dev/cosign-test-365209/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753 --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - The signatures were verified against the specified public key
[
  {
    "critical": {
      "identity": {
        "docker-reference": "us-central1-docker.pkg.dev/cosign-test-365209/repo1/myimage"
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
COSIGN_EXPERIMENTAL=1  cosign verify  us-central1-docker.pkg.dev/$PROJECT_ID/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753 | jq '.'
```

gives

```text
Verification for us-central1-docker.pkg.dev/cosign-test-365209/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753 --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - Existence of the claims in the transparency log was verified offline
  - Any certificates were verified against the Fulcio roots.
[
  {
    "critical": {
      "identity": {
        "docker-reference": "us-central1-docker.pkg.dev/cosign-test-365209/repo1/myimage"
      },
      "image": {
        "docker-manifest-digest": "sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753"
      },
      "type": "cosign container image signature"
    },
    "optional": {
      "Bundle": {
        "SignedEntryTimestamp": "MEUCIGHZnUTc/IllmCsm0l/UYzxqyWWAcqMvPDM+uN6pNhBJAiEA/wZQxTTrjg76HMH5HnUU422/3MyDOgkfxVlEpRlHUYM=",
        "Payload": {
          "body": "eyJhcGlWZXJzaW9uIjoiMC4wLjEiLCJraW5kIjoiaGFzaGVkcmVrb3JkIiwic3BlYyI6eyJkYXRhIjp7Imhhc2giOnsiYWxnb3JpdGhtIjoic2hhMjU2IiwidmFsdWUiOiI2YmJiZWRmMzRkN2EwMGU4YmUyOWFkYzlmNDczNzAyYzU3YmNlMjYyMmUxN2E1MWVjZDQyYTA1NDBkOWVlMDZmIn19LCJzaWduYXR1cmUiOnsiY29udGVudCI6Ik1FVUNJUURRNGJmTTNRcy92cUUydk4wV0UvOXJySWFsYkdvZ3M3RDQ3VnJ1c3MzVVZnSWdScm9odWtHWW1Hd3Mvb2c1UWFhdDI1WGdTK3B6NzNWWkQzRTI2eWJOSGpjPSIsInB1YmxpY0tleSI6eyJjb250ZW50IjoiTFMwdExTMUNSVWRKVGlCRFJWSlVTVVpKUTBGVVJTMHRMUzB0Q2sxSlNVTjJWRU5EUVd0UFowRjNTVUpCWjBsVlduRmhPRGhHZG5Ca2RVNXFOR1V6SzJKT2RWZDBjMXBSYVVsQmQwTm5XVWxMYjFwSmVtb3dSVUYzVFhjS1RucEZWazFDVFVkQk1WVkZRMmhOVFdNeWJHNWpNMUoyWTIxVmRWcEhWakpOVWpSM1NFRlpSRlpSVVVSRmVGWjZZVmRrZW1SSE9YbGFVekZ3WW01U2JBcGpiVEZzV2tkc2FHUkhWWGRJYUdOT1RXcEplRTFFUlhoTlJHc3hUV3BKTTFkb1kwNU5ha2w0VFVSRmVFMVVRWGROYWtrelYycEJRVTFHYTNkRmQxbElDa3R2V2tsNmFqQkRRVkZaU1V0dldrbDZhakJFUVZGalJGRm5RVVZyTkhoYU9HdFVlREJOV0RCVVJVVkplVUZ2TnpoM09XMHdNVWx6UkZGaVZqRlBhVXNLV1ZjNFRrUlplRWg2Y21oWmEwNHJheXRuVWxkdlNGSjNPV3RETlhCMmFrbGtiakpvWTJ0c01uQldNV1kwY1dsMmNHRlBRMEZYU1hkblowWmxUVUUwUndwQk1WVmtSSGRGUWk5M1VVVkJkMGxJWjBSQlZFSm5UbFpJVTFWRlJFUkJTMEpuWjNKQ1owVkdRbEZqUkVGNlFXUkNaMDVXU0ZFMFJVWm5VVlZPV2xkWkNsWllla05MTDB0VlVHeEpjRUZoTldSUWRHbExRbFpCZDBoM1dVUldVakJxUWtKbmQwWnZRVlV6T1ZCd2VqRlphMFZhWWpWeFRtcHdTMFpYYVhocE5Ga0tXa1E0ZDFCM1dVUldVakJTUVZGSUwwSkVWWGROTkVWNFdUSTVlbUZYWkhWUlIwNTJZekpzYm1KcE1UQmFXRTR3VEZSTk1rNVVTWGRQVXpWd1dWY3dkUXBhTTA1c1kyNWFjRmt5Vm1oWk1rNTJaRmMxTUV4dFRuWmlWRUZ3UW1kdmNrSm5SVVZCV1U4dlRVRkZRa0pDZEc5a1NGSjNZM3B2ZGt3eVJtcFpNamt4Q21KdVVucE1iV1IyWWpKa2MxcFROV3BpTWpCM1oxbHZSME5wYzBkQlVWRkNNVzVyUTBKQlNVVm1RVkkyUVVoblFXUm5RVWxaU2t4M1MwWk1MMkZGV0ZJS01GZHpibWhLZUVaYWVHbHpSbW96UkU5T1NuUTFjbmRwUW1wYWRtTm5RVUZCV1ZCSFpHSnJWVUZCUVVWQmQwSklUVVZWUTBsUlEzVldTWGQ1WW0xUmRRcHJWSHBKTkhwS2NHeHdVbXR2VkRRemREVmlkMVJHY1RSUU1XSm9WMDU0Y0RGQlNXZFZlVTU0WTA5Q2RERkJWMjl5Y3pKU1NIUm5TbnBLVkVnMmFXSTJDbEVyVUdSVmIwYzNjbmwwWjBkSGMzZERaMWxKUzI5YVNYcHFNRVZCZDAxRVlVRkJkMXBSU1hkVmIxVklaek5rVjBWT2IxVnRXRWczT1U5WlVURXhXa01LY0M5cVdtUXlNbEJHYlVOblRtbERSRTFwVEdwMVQyRnJSMWgzT0hkVVVHVTBUVkpxUVV0VE1rRnFSVUUxWkVOdGNVVjFPVnBwVG5wWlZIQkpNMlpCWlFwemJHdzRZMlZ6TXpseGFHVm5jMDh5WlV4NFdYSXJhM0JZYTFoUE5GcDRVMmt3UTNZcmNsSkZiV2xLWndvdExTMHRMVVZPUkNCRFJWSlVTVVpKUTBGVVJTMHRMUzB0Q2c9PSJ9fX19",
          "integratedTime": 1665481948,
          "logIndex": 4884681,
          "logID": "c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d"
        }
      },
      "Issuer": "https://accounts.google.com",
      "Subject": "cosign@cosign-test-365209.iam.gserviceaccount.com",
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
        "value": "6bbbedf34d7a00e8be29adc9f473702c57bce2622e17a51ecd42a0540d9ee06f"
      }
    },
    "signature": {
      "content": "MEUCIQDQ4bfM3Qs/vqE2vN0WE/9rrIalbGogs7D47Vruss3UVgIgRrohukGYmGws/og5Qaat25XgS+pz73VZD3E26ybNHjc=",
      "publicKey": {
        "content": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUN2VENDQWtPZ0F3SUJBZ0lVWnFhODhGdnBkdU5qNGUzK2JOdVd0c1pRaUlBd0NnWUlLb1pJemowRUF3TXcKTnpFVk1CTUdBMVVFQ2hNTWMybG5jM1J2Y21VdVpHVjJNUjR3SEFZRFZRUURFeFZ6YVdkemRHOXlaUzFwYm5SbApjbTFsWkdsaGRHVXdIaGNOTWpJeE1ERXhNRGsxTWpJM1doY05Nakl4TURFeE1UQXdNakkzV2pBQU1Ga3dFd1lICktvWkl6ajBDQVFZSUtvWkl6ajBEQVFjRFFnQUVrNHhaOGtUeDBNWDBURUVJeUFvNzh3OW0wMUlzRFFiVjFPaUsKWVc4TkRZeEh6cmhZa04raytnUldvSFJ3OWtDNXB2aklkbjJoY2tsMnBWMWY0cWl2cGFPQ0FXSXdnZ0ZlTUE0RwpBMVVkRHdFQi93UUVBd0lIZ0RBVEJnTlZIU1VFRERBS0JnZ3JCZ0VGQlFjREF6QWRCZ05WSFE0RUZnUVVOWldZClZYekNLL0tVUGxJcEFhNWRQdGlLQlZBd0h3WURWUjBqQkJnd0ZvQVUzOVBwejFZa0VaYjVxTmpwS0ZXaXhpNFkKWkQ4d1B3WURWUjBSQVFIL0JEVXdNNEV4WTI5emFXZHVRR052YzJsbmJpMTBaWE4wTFRNMk5USXdPUzVwWVcwdQpaM05sY25acFkyVmhZMk52ZFc1MExtTnZiVEFwQmdvckJnRUVBWU8vTUFFQkJCdG9kSFJ3Y3pvdkwyRmpZMjkxCmJuUnpMbWR2YjJkc1pTNWpiMjB3Z1lvR0Npc0dBUVFCMW5rQ0JBSUVmQVI2QUhnQWRnQUlZSkx3S0ZML2FFWFIKMFdzbmhKeEZaeGlzRmozRE9OSnQ1cndpQmpadmNnQUFBWVBHZGJrVUFBQUVBd0JITUVVQ0lRQ3VWSXd5Ym1RdQprVHpJNHpKcGxwUmtvVDQzdDVid1RGcTRQMWJoV054cDFBSWdVeU54Y09CdDFBV29yczJSSHRnSnpKVEg2aWI2ClErUGRVb0c3cnl0Z0dHc3dDZ1lJS29aSXpqMEVBd01EYUFBd1pRSXdVb1VIZzNkV0VOb1VtWEg3OU9ZUTExWkMKcC9qWmQyMlBGbUNnTmlDRE1pTGp1T2FrR1h3OHdUUGU0TVJqQUtTMkFqRUE1ZENtcUV1OVppTnpZVHBJM2ZBZQpzbGw4Y2VzMzlxaGVnc08yZUx4WXIra3BYa1hPNFp4U2kwQ3YrclJFbWlKZwotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
      }
    }
  }
}
```

from there the base64encoded `publicKey` is what was issued during the [signing ceremony](https://docs.sigstore.dev/fulcio/certificate-issuing-overview). 

```
-----BEGIN CERTIFICATE-----
MIICvTCCAkOgAwIBAgIUZqa88FvpduNj4e3+bNuWtsZQiIAwCgYIKoZIzj0EAwMw
NzEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MR4wHAYDVQQDExVzaWdzdG9yZS1pbnRl
cm1lZGlhdGUwHhcNMjIxMDExMDk1MjI3WhcNMjIxMDExMTAwMjI3WjAAMFkwEwYH
KoZIzj0CAQYIKoZIzj0DAQcDQgAEk4xZ8kTx0MX0TEEIyAo78w9m01IsDQbV1OiK
YW8NDYxHzrhYkN+k+gRWoHRw9kC5pvjIdn2hckl2pV1f4qivpaOCAWIwggFeMA4G
A1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAdBgNVHQ4EFgQUNZWY
VXzCK/KUPlIpAa5dPtiKBVAwHwYDVR0jBBgwFoAU39Ppz1YkEZb5qNjpKFWixi4Y
ZD8wPwYDVR0RAQH/BDUwM4ExY29zaWduQGNvc2lnbi10ZXN0LTM2NTIwOS5pYW0u
Z3NlcnZpY2VhY2NvdW50LmNvbTApBgorBgEEAYO/MAEBBBtodHRwczovL2FjY291
bnRzLmdvb2dsZS5jb20wgYoGCisGAQQB1nkCBAIEfAR6AHgAdgAIYJLwKFL/aEXR
0WsnhJxFZxisFj3DONJt5rwiBjZvcgAAAYPGdbkUAAAEAwBHMEUCIQCuVIwybmQu
kTzI4zJplpRkoT43t5bwTFq4P1bhWNxp1AIgUyNxcOBt1AWors2RHtgJzJTH6ib6
Q+PdUoG7rytgGGswCgYIKoZIzj0EAwMDaAAwZQIwUoUHg3dWENoUmXH79OYQ11ZC
p/jZd22PFmCgNiCDMiLjuOakGXw8wTPe4MRjAKS2AjEA5dCmqEu9ZiNzYTpI3fAe
sll8ces39qhegsO2eLxYr+kpXkXO4ZxSi0Cv+rREmiJg
-----END CERTIFICATE-----
```

which expanded is 

```bash
$  openssl x509 -in cosign.crt -noout -text

Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number:
            66:a6:bc:f0:5b:e9:76:e3:63:e1:ed:fe:6c:db:96:b6:c6:50:88:80
        Signature Algorithm: ecdsa-with-SHA384
        Issuer: O = sigstore.dev, CN = sigstore-intermediate
        Validity
            Not Before: Oct 11 09:52:27 2022 GMT
            Not After : Oct 11 10:02:27 2022 GMT
        Subject: 
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
                Public-Key: (256 bit)
                pub:
                    04:93:8c:59:f2:44:f1:d0:c5:f4:4c:41:08:c8:0a:
                    3b:f3:0f:66:d3:52:2c:0d:06:d5:d4:e8:8a:61:6f:
                    0d:0d:8c:47:ce:b8:58:90:df:a4:fa:04:56:a0:74:
                    70:f6:40:b9:a6:f8:c8:76:7d:a1:72:49:76:a5:5d:
                    5f:e2:a8:af:a5
                ASN1 OID: prime256v1
                NIST CURVE: P-256
        X509v3 extensions:
            X509v3 Key Usage: critical
                Digital Signature
            X509v3 Extended Key Usage: 
                Code Signing
            X509v3 Subject Key Identifier: 
                35:95:98:55:7C:C2:2B:F2:94:3E:52:29:01:AE:5D:3E:D8:8A:05:50
            X509v3 Authority Key Identifier: 
                DF:D3:E9:CF:56:24:11:96:F9:A8:D8:E9:28:55:A2:C6:2E:18:64:3F
            X509v3 Subject Alternative Name: critical
                email:cosign@cosign-test-365209.iam.gserviceaccount.com  <<<<<<<<<<<<
            1.3.6.1.4.1.57264.1.1: 
                https://accounts.google.com
            CT Precertificate SCTs: 
                Signed Certificate Timestamp:
                    Version   : v1 (0x0)
                    Log ID    : 08:60:92:F0:28:52:FF:68:45:D1:D1:6B:27:84:9C:45:
                                67:18:AC:16:3D:C3:38:D2:6D:E6:BC:22:06:36:6F:72
                    Timestamp : Oct 11 09:52:27.412 2022 GMT
                    Extensions: none
                    Signature : ecdsa-with-SHA256
                                30:45:02:21:00:AE:54:8C:32:6E:64:2E:91:3C:C8:E3:
                                32:69:96:94:64:A1:3E:37:B7:96:F0:4C:5A:B8:3F:56:
                                E1:58:DC:69:D4:02:20:53:23:71:70:E0:6D:D4:05:A8:
                                AE:CD:91:1E:D8:09:CC:94:C7:EA:26:FA:43:E3:DD:52:
                                81:BB:AF:2B:60:18:6B
    Signature Algorithm: ecdsa-with-SHA384
```

NOTE the OID `1.3.6.1.4.1.57264.1.1` is registered to [here](https://github.com/sigstore/fulcio/blob/main/docs/oid-info.md#directory) and denotes the OIDC Token's isuser

Now use `rekor-cli` to search for what we added to the transparency log using


* `sha` value from `hashedrekord`

```bash
$ rekor-cli search --rekor_server https://rekor.sigstore.dev \
   --sha  6bbbedf34d7a00e8be29adc9f473702c57bce2622e17a51ecd42a0540d9ee06f

  Found matching entries (listed by UUID):
  24296fb24b8ad77ae8a9e596d1cc25b01df1b03be9b9f2d91cf971ff04e5d1ada53b5ce55278425b
```

* the email for the build service account's `OIDC`

```bash
$ rekor-cli search --rekor_server https://rekor.sigstore.dev  --email cosign@$PROJECT_ID.iam.gserviceaccount.com

Found matching entries (listed by UUID):
24296fb24b8ad77ac921dde21b687cb74c5d447bc22be1695e0f8a804d6b9d4e5678f02f354984fc
24296fb24b8ad77ae8a9e596d1cc25b01df1b03be9b9f2d91cf971ff04e5d1ada53b5ce55278425b
```

note each `UUID` asserts something different:  the `signature` and another one for the `attestation`


For the `Signature`

```
rekor-cli get --rekor_server https://rekor.sigstore.dev  \
  --uuid 24296fb24b8ad77ae8a9e596d1cc25b01df1b03be9b9f2d91cf971ff04e5d1ada53b5ce55278425b
```

outputs (note the `Index` value matches what we have in the "sign_oidc" build step)

```text
LogID: c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d
Index: 4884681
IntegratedTime: 2022-10-11T09:52:28Z
UUID: 24296fb24b8ad77ae8a9e596d1cc25b01df1b03be9b9f2d91cf971ff04e5d1ada53b5ce55278425b
Body: {
  "HashedRekordObj": {
    "data": {
      "hash": {
        "algorithm": "sha256",
        "value": "6bbbedf34d7a00e8be29adc9f473702c57bce2622e17a51ecd42a0540d9ee06f"
      }
    },
    "signature": {
      "content": "MEUCIQDQ4bfM3Qs/vqE2vN0WE/9rrIalbGogs7D47Vruss3UVgIgRrohukGYmGws/og5Qaat25XgS+pz73VZD3E26ybNHjc=",
      "publicKey": {
        "content": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUN2VENDQWtPZ0F3SUJBZ0lVWnFhODhGdnBkdU5qNGUzK2JOdVd0c1pRaUlBd0NnWUlLb1pJemowRUF3TXcKTnpFVk1CTUdBMVVFQ2hNTWMybG5jM1J2Y21VdVpHVjJNUjR3SEFZRFZRUURFeFZ6YVdkemRHOXlaUzFwYm5SbApjbTFsWkdsaGRHVXdIaGNOTWpJeE1ERXhNRGsxTWpJM1doY05Nakl4TURFeE1UQXdNakkzV2pBQU1Ga3dFd1lICktvWkl6ajBDQVFZSUtvWkl6ajBEQVFjRFFnQUVrNHhaOGtUeDBNWDBURUVJeUFvNzh3OW0wMUlzRFFiVjFPaUsKWVc4TkRZeEh6cmhZa04raytnUldvSFJ3OWtDNXB2aklkbjJoY2tsMnBWMWY0cWl2cGFPQ0FXSXdnZ0ZlTUE0RwpBMVVkRHdFQi93UUVBd0lIZ0RBVEJnTlZIU1VFRERBS0JnZ3JCZ0VGQlFjREF6QWRCZ05WSFE0RUZnUVVOWldZClZYekNLL0tVUGxJcEFhNWRQdGlLQlZBd0h3WURWUjBqQkJnd0ZvQVUzOVBwejFZa0VaYjVxTmpwS0ZXaXhpNFkKWkQ4d1B3WURWUjBSQVFIL0JEVXdNNEV4WTI5emFXZHVRR052YzJsbmJpMTBaWE4wTFRNMk5USXdPUzVwWVcwdQpaM05sY25acFkyVmhZMk52ZFc1MExtTnZiVEFwQmdvckJnRUVBWU8vTUFFQkJCdG9kSFJ3Y3pvdkwyRmpZMjkxCmJuUnpMbWR2YjJkc1pTNWpiMjB3Z1lvR0Npc0dBUVFCMW5rQ0JBSUVmQVI2QUhnQWRnQUlZSkx3S0ZML2FFWFIKMFdzbmhKeEZaeGlzRmozRE9OSnQ1cndpQmpadmNnQUFBWVBHZGJrVUFBQUVBd0JITUVVQ0lRQ3VWSXd5Ym1RdQprVHpJNHpKcGxwUmtvVDQzdDVid1RGcTRQMWJoV054cDFBSWdVeU54Y09CdDFBV29yczJSSHRnSnpKVEg2aWI2ClErUGRVb0c3cnl0Z0dHc3dDZ1lJS29aSXpqMEVBd01EYUFBd1pRSXdVb1VIZzNkV0VOb1VtWEg3OU9ZUTExWkMKcC9qWmQyMlBGbUNnTmlDRE1pTGp1T2FrR1h3OHdUUGU0TVJqQUtTMkFqRUE1ZENtcUV1OVppTnpZVHBJM2ZBZQpzbGw4Y2VzMzlxaGVnc08yZUx4WXIra3BYa1hPNFp4U2kwQ3YrclJFbWlKZwotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
      }
    }
  }
}
```

the certificate from the decoded publicKey outputs the cert issued by falcio

```
-----BEGIN CERTIFICATE-----
MIICvTCCAkOgAwIBAgIUZqa88FvpduNj4e3+bNuWtsZQiIAwCgYIKoZIzj0EAwMw
NzEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MR4wHAYDVQQDExVzaWdzdG9yZS1pbnRl
cm1lZGlhdGUwHhcNMjIxMDExMDk1MjI3WhcNMjIxMDExMTAwMjI3WjAAMFkwEwYH
KoZIzj0CAQYIKoZIzj0DAQcDQgAEk4xZ8kTx0MX0TEEIyAo78w9m01IsDQbV1OiK
YW8NDYxHzrhYkN+k+gRWoHRw9kC5pvjIdn2hckl2pV1f4qivpaOCAWIwggFeMA4G
A1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAdBgNVHQ4EFgQUNZWY
VXzCK/KUPlIpAa5dPtiKBVAwHwYDVR0jBBgwFoAU39Ppz1YkEZb5qNjpKFWixi4Y
ZD8wPwYDVR0RAQH/BDUwM4ExY29zaWduQGNvc2lnbi10ZXN0LTM2NTIwOS5pYW0u
Z3NlcnZpY2VhY2NvdW50LmNvbTApBgorBgEEAYO/MAEBBBtodHRwczovL2FjY291
bnRzLmdvb2dsZS5jb20wgYoGCisGAQQB1nkCBAIEfAR6AHgAdgAIYJLwKFL/aEXR
0WsnhJxFZxisFj3DONJt5rwiBjZvcgAAAYPGdbkUAAAEAwBHMEUCIQCuVIwybmQu
kTzI4zJplpRkoT43t5bwTFq4P1bhWNxp1AIgUyNxcOBt1AWors2RHtgJzJTH6ib6
Q+PdUoG7rytgGGswCgYIKoZIzj0EAwMDaAAwZQIwUoUHg3dWENoUmXH79OYQ11ZC
p/jZd22PFmCgNiCDMiLjuOakGXw8wTPe4MRjAKS2AjEA5dCmqEu9ZiNzYTpI3fAe
sll8ces39qhegsO2eLxYr+kpXkXO4ZxSi0Cv+rREmiJg
-----END CERTIFICATE-----




Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number:
            66:a6:bc:f0:5b:e9:76:e3:63:e1:ed:fe:6c:db:96:b6:c6:50:88:80
        Signature Algorithm: ecdsa-with-SHA384
        Issuer: O = sigstore.dev, CN = sigstore-intermediate
        Validity
            Not Before: Oct 11 09:52:27 2022 GMT
            Not After : Oct 11 10:02:27 2022 GMT
        Subject: 
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
                Public-Key: (256 bit)
                pub:
                    04:93:8c:59:f2:44:f1:d0:c5:f4:4c:41:08:c8:0a:
                    3b:f3:0f:66:d3:52:2c:0d:06:d5:d4:e8:8a:61:6f:
                    0d:0d:8c:47:ce:b8:58:90:df:a4:fa:04:56:a0:74:
                    70:f6:40:b9:a6:f8:c8:76:7d:a1:72:49:76:a5:5d:
                    5f:e2:a8:af:a5
                ASN1 OID: prime256v1
                NIST CURVE: P-256
        X509v3 extensions:
            X509v3 Key Usage: critical
                Digital Signature
            X509v3 Extended Key Usage: 
                Code Signing
            X509v3 Subject Key Identifier: 
                35:95:98:55:7C:C2:2B:F2:94:3E:52:29:01:AE:5D:3E:D8:8A:05:50
            X509v3 Authority Key Identifier: 
                DF:D3:E9:CF:56:24:11:96:F9:A8:D8:E9:28:55:A2:C6:2E:18:64:3F
            X509v3 Subject Alternative Name: critical
                email:cosign@cosign-test-365209.iam.gserviceaccount.com  <<<<<<<<<<<<<
            1.3.6.1.4.1.57264.1.1: 
                https://accounts.google.com
            CT Precertificate SCTs: 
                Signed Certificate Timestamp:
                    Version   : v1 (0x0)
                    Log ID    : 08:60:92:F0:28:52:FF:68:45:D1:D1:6B:27:84:9C:45:
                                67:18:AC:16:3D:C3:38:D2:6D:E6:BC:22:06:36:6F:72
                    Timestamp : Oct 11 09:52:27.412 2022 GMT
                    Extensions: none
                    Signature : ecdsa-with-SHA256
                                30:45:02:21:00:AE:54:8C:32:6E:64:2E:91:3C:C8:E3:
                                32:69:96:94:64:A1:3E:37:B7:96:F0:4C:5A:B8:3F:56:
                                E1:58:DC:69:D4:02:20:53:23:71:70:E0:6D:D4:05:A8:
                                AE:CD:91:1E:D8:09:CC:94:C7:EA:26:FA:43:E3:DD:52:
                                81:BB:AF:2B:60:18:6B
    Signature Algorithm: ecdsa-with-SHA384

```


for the `Attestation`

```bash
rekor-cli get --rekor_server https://rekor.sigstore.dev \
   --uuid 24296fb24b8ad77ac921dde21b687cb74c5d447bc22be1695e0f8a804d6b9d4e5678f02f354984fc
```

gives (again, note the attestations and `Index` that matches the "attest_oidc" step)

```text
LogID: c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d
Attestation: {"_type":"https://in-toto.io/Statement/v0.1","predicateType":"cosign.sigstore.dev/attestation/v1","subject":[{"name":"us-central1-docker.pkg.dev/cosign-test-365209/repo1/myimage","digest":{"sha256":"83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753"}}],"predicate":{"Data":"{ \"projectid\": \"cosign-test-365209\", \"buildid\": \"0594f2e5-f600-48db-8aa2-daf58ee1acb5\", \"foo\":\"bar\", \"commitsha\": \"dd93e9e893ffaf2c4cafeb2e534bf03f66d7bf28\" }","Timestamp":"2022-10-11T09:52:30Z"}}
Index: 4884684
IntegratedTime: 2022-10-11T09:52:30Z
UUID: 24296fb24b8ad77ac921dde21b687cb74c5d447bc22be1695e0f8a804d6b9d4e5678f02f354984fc
Body: {
  "IntotoObj": {
    "content": {
      "hash": {
        "algorithm": "sha256",
        "value": "0f423022b98d7a9147d52708e1c0cfa314e1825da4e7c1acfec6b37b463c6fc9"
      },
      "payloadHash": {
        "algorithm": "sha256",
        "value": "66c1bccfea7611934df3b67cf6c660d0120fbad40bcbccad4db940d463e62188"
      }
    },
    "publicKey": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUN2VENDQWtPZ0F3SUJBZ0lVUEZzMXZ2S1pwVFRMMlFvQlJNTU1vQmVub3N3d0NnWUlLb1pJemowRUF3TXcKTnpFVk1CTUdBMVVFQ2hNTWMybG5jM1J2Y21VdVpHVjJNUjR3SEFZRFZRUURFeFZ6YVdkemRHOXlaUzFwYm5SbApjbTFsWkdsaGRHVXdIaGNOTWpJeE1ERXhNRGsxTWpJNVdoY05Nakl4TURFeE1UQXdNakk1V2pBQU1Ga3dFd1lICktvWkl6ajBDQVFZSUtvWkl6ajBEQVFjRFFnQUVVcmpDSjFhMjFBNWUrOFQrUDFTYjh0QjRrejZ0L1hXUWZwYVQKK0x1RXZtSXFLc2VNUGlOQ1hsUTBjdUpETUJDUFZPdnF6R0dQWGwyendRU0JaRnRPc0tPQ0FXSXdnZ0ZlTUE0RwpBMVVkRHdFQi93UUVBd0lIZ0RBVEJnTlZIU1VFRERBS0JnZ3JCZ0VGQlFjREF6QWRCZ05WSFE0RUZnUVVtQmo1CnNjdnR4WURjcmRYT0N6b0NkeXB0ajUwd0h3WURWUjBqQkJnd0ZvQVUzOVBwejFZa0VaYjVxTmpwS0ZXaXhpNFkKWkQ4d1B3WURWUjBSQVFIL0JEVXdNNEV4WTI5emFXZHVRR052YzJsbmJpMTBaWE4wTFRNMk5USXdPUzVwWVcwdQpaM05sY25acFkyVmhZMk52ZFc1MExtTnZiVEFwQmdvckJnRUVBWU8vTUFFQkJCdG9kSFJ3Y3pvdkwyRmpZMjkxCmJuUnpMbWR2YjJkc1pTNWpiMjB3Z1lvR0Npc0dBUVFCMW5rQ0JBSUVmQVI2QUhnQWRnQUlZSkx3S0ZML2FFWFIKMFdzbmhKeEZaeGlzRmozRE9OSnQ1cndpQmpadmNnQUFBWVBHZGNJSEFBQUVBd0JITUVVQ0lFUndFMExRWERKawo3R2VpcTREOXZwbnNjbWhMNUkvaWNFdW4ra1A1L3B4S0FpRUF1eEpTTjR6UGxZcWQ3aGJkSXRjdUVMajNiL2pjCjcxOHBEL3k2b281eTdsVXdDZ1lJS29aSXpqMEVBd01EYUFBd1pRSXhBTXQwUXBFZVFyam9vRXg3TEtMVXRTamEKTUNDenFjK1FrZDFpMkR4ZVQ2Tm9iNm9xbzlpendPVUVwT0Rna1ByZDBnSXdibnlMbWREZXRxN2pTcks5NGVwNAowbm83QmNuczc0MDROWGN2WFpNR1FpNjRwbVRIVW1jMHVRSXhyMDBWU0dWZgotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="
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
            3c:5b:35:be:f2:99:a5:34:cb:d9:0a:01:44:c3:0c:a0:17:a7:a2:cc
        Signature Algorithm: ecdsa-with-SHA384
        Issuer: O = sigstore.dev, CN = sigstore-intermediate
        Validity
            Not Before: Oct 11 09:52:29 2022 GMT
            Not After : Oct 11 10:02:29 2022 GMT
        Subject: 
        Subject Public Key Info:
            Public Key Algorithm: id-ecPublicKey
                Public-Key: (256 bit)
                pub:
                    04:52:b8:c2:27:56:b6:d4:0e:5e:fb:c4:fe:3f:54:
                    9b:f2:d0:78:93:3e:ad:fd:75:90:7e:96:93:f8:bb:
                    84:be:62:2a:2a:c7:8c:3e:23:42:5e:54:34:72:e2:
                    43:30:10:8f:54:eb:ea:cc:61:8f:5e:5d:b3:c1:04:
                    81:64:5b:4e:b0
                ASN1 OID: prime256v1
                NIST CURVE: P-256
        X509v3 extensions:
            X509v3 Key Usage: critical
                Digital Signature
            X509v3 Extended Key Usage: 
                Code Signing
            X509v3 Subject Key Identifier: 
                98:18:F9:B1:CB:ED:C5:80:DC:AD:D5:CE:0B:3A:02:77:2A:6D:8F:9D
            X509v3 Authority Key Identifier: 
                DF:D3:E9:CF:56:24:11:96:F9:A8:D8:E9:28:55:A2:C6:2E:18:64:3F
            X509v3 Subject Alternative Name: critical
                email:cosign@cosign-test-365209.iam.gserviceaccount.com
            1.3.6.1.4.1.57264.1.1: 
                https://accounts.google.com
            CT Precertificate SCTs: 
                Signed Certificate Timestamp:
                    Version   : v1 (0x0)
                    Log ID    : 08:60:92:F0:28:52:FF:68:45:D1:D1:6B:27:84:9C:45:
                                67:18:AC:16:3D:C3:38:D2:6D:E6:BC:22:06:36:6F:72
                    Timestamp : Oct 11 09:52:29.703 2022 GMT
                    Extensions: none
                    Signature : ecdsa-with-SHA256
                                30:45:02:20:44:70:13:42:D0:5C:32:64:EC:67:A2:AB:
                                80:FD:BE:99:EC:72:68:4B:E4:8F:E2:70:4B:A7:FA:43:
                                F9:FE:9C:4A:02:21:00:BB:12:52:37:8C:CF:95:8A:9D:
                                EE:16:DD:22:D7:2E:10:B8:F7:6F:F8:DC:EF:5F:29:0F:
                                FC:BA:A2:8E:72:EE:55
    Signature Algorithm: ecdsa-with-SHA384

```


(just note the timestamps on the two certs are like 2s apart (which was about when the build steps happened for each step;  also note the email SAN is the sam))


### Crane

You can also view the registry manifest for the signature using [crane](github.com/google/go-containerregistry/cmd/crane)

You'll see the two signatures (one for KMS, another larger one for the OIDC signature metadata)

```text
# go install github.com/google/go-containerregistry/cmd/crane@latest

$ crane  manifest us-central1-docker.pkg.dev/$PROJECT_ID/repo1/myimage:sha256-83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753.sig | jq '.'


{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": {
    "mediaType": "application/vnd.oci.image.config.v1+json",
    "size": 352,
    "digest": "sha256:985d55dde83f5fd0390dd4a22c3bfcb5d0d2983a432ece395550f2906f06c530"
  },
  "layers": [
    {
      "mediaType": "application/vnd.dev.cosign.simplesigning.v1+json",
      "size": 288,
      "digest": "sha256:6bbbedf34d7a00e8be29adc9f473702c57bce2622e17a51ecd42a0540d9ee06f",
      "annotations": {
        "dev.cosignproject.cosign/signature": "MEUCIBpi56lVbUtelTvHfPSX+2pubeYSMUEV+SsXjUR2eKmoAiEA3/27jrQIfDr1kG+5omzBIVUXoAgPlKMYGkM1vA/Oi8Y="
      }
    },
    {
      "mediaType": "application/vnd.dev.cosign.simplesigning.v1+json",
      "size": 288,
      "digest": "sha256:6bbbedf34d7a00e8be29adc9f473702c57bce2622e17a51ecd42a0540d9ee06f",
      "annotations": {
        "dev.cosignproject.cosign/signature": "MEUCIQDQ4bfM3Qs/vqE2vN0WE/9rrIalbGogs7D47Vruss3UVgIgRrohukGYmGws/og5Qaat25XgS+pz73VZD3E26ybNHjc=",
        "dev.sigstore.cosign/bundle": "{\"SignedEntryTimestamp\":\"MEUCIGHZnUTc/IllmCsm0l/UYzxqyWWAcqMvPDM+uN6pNhBJAiEA/wZQxTTrjg76HMH5HnUU422/3MyDOgkfxVlEpRlHUYM=\",\"Payload\":{\"body\":\"eyJhcGlWZXJzaW9uIjoiMC4wLjEiLCJraW5kIjoiaGFzaGVkcmVrb3JkIiwic3BlYyI6eyJkYXRhIjp7Imhhc2giOnsiYWxnb3JpdGhtIjoic2hhMjU2IiwidmFsdWUiOiI2YmJiZWRmMzRkN2EwMGU4YmUyOWFkYzlmNDczNzAyYzU3YmNlMjYyMmUxN2E1MWVjZDQyYTA1NDBkOWVlMDZmIn19LCJzaWduYXR1cmUiOnsiY29udGVudCI6Ik1FVUNJUURRNGJmTTNRcy92cUUydk4wV0UvOXJySWFsYkdvZ3M3RDQ3VnJ1c3MzVVZnSWdScm9odWtHWW1Hd3Mvb2c1UWFhdDI1WGdTK3B6NzNWWkQzRTI2eWJOSGpjPSIsInB1YmxpY0tleSI6eyJjb250ZW50IjoiTFMwdExTMUNSVWRKVGlCRFJWSlVTVVpKUTBGVVJTMHRMUzB0Q2sxSlNVTjJWRU5EUVd0UFowRjNTVUpCWjBsVlduRmhPRGhHZG5Ca2RVNXFOR1V6SzJKT2RWZDBjMXBSYVVsQmQwTm5XVWxMYjFwSmVtb3dSVUYzVFhjS1RucEZWazFDVFVkQk1WVkZRMmhOVFdNeWJHNWpNMUoyWTIxVmRWcEhWakpOVWpSM1NFRlpSRlpSVVVSRmVGWjZZVmRrZW1SSE9YbGFVekZ3WW01U2JBcGpiVEZzV2tkc2FHUkhWWGRJYUdOT1RXcEplRTFFUlhoTlJHc3hUV3BKTTFkb1kwNU5ha2w0VFVSRmVFMVVRWGROYWtrelYycEJRVTFHYTNkRmQxbElDa3R2V2tsNmFqQkRRVkZaU1V0dldrbDZhakJFUVZGalJGRm5RVVZyTkhoYU9HdFVlREJOV0RCVVJVVkplVUZ2TnpoM09XMHdNVWx6UkZGaVZqRlBhVXNLV1ZjNFRrUlplRWg2Y21oWmEwNHJheXRuVWxkdlNGSjNPV3RETlhCMmFrbGtiakpvWTJ0c01uQldNV1kwY1dsMmNHRlBRMEZYU1hkblowWmxUVUUwUndwQk1WVmtSSGRGUWk5M1VVVkJkMGxJWjBSQlZFSm5UbFpJVTFWRlJFUkJTMEpuWjNKQ1owVkdRbEZqUkVGNlFXUkNaMDVXU0ZFMFJVWm5VVlZPV2xkWkNsWllla05MTDB0VlVHeEpjRUZoTldSUWRHbExRbFpCZDBoM1dVUldVakJxUWtKbmQwWnZRVlV6T1ZCd2VqRlphMFZhWWpWeFRtcHdTMFpYYVhocE5Ga0tXa1E0ZDFCM1dVUldVakJTUVZGSUwwSkVWWGROTkVWNFdUSTVlbUZYWkhWUlIwNTJZekpzYm1KcE1UQmFXRTR3VEZSTk1rNVVTWGRQVXpWd1dWY3dkUXBhTTA1c1kyNWFjRmt5Vm1oWk1rNTJaRmMxTUV4dFRuWmlWRUZ3UW1kdmNrSm5SVVZCV1U4dlRVRkZRa0pDZEc5a1NGSjNZM3B2ZGt3eVJtcFpNamt4Q21KdVVucE1iV1IyWWpKa2MxcFROV3BpTWpCM1oxbHZSME5wYzBkQlVWRkNNVzVyUTBKQlNVVm1RVkkyUVVoblFXUm5RVWxaU2t4M1MwWk1MMkZGV0ZJS01GZHpibWhLZUVaYWVHbHpSbW96UkU5T1NuUTFjbmRwUW1wYWRtTm5RVUZCV1ZCSFpHSnJWVUZCUVVWQmQwSklUVVZWUTBsUlEzVldTWGQ1WW0xUmRRcHJWSHBKTkhwS2NHeHdVbXR2VkRRemREVmlkMVJHY1RSUU1XSm9WMDU0Y0RGQlNXZFZlVTU0WTA5Q2RERkJWMjl5Y3pKU1NIUm5TbnBLVkVnMmFXSTJDbEVyVUdSVmIwYzNjbmwwWjBkSGMzZERaMWxKUzI5YVNYcHFNRVZCZDAxRVlVRkJkMXBSU1hkVmIxVklaek5rVjBWT2IxVnRXRWczT1U5WlVURXhXa01LY0M5cVdtUXlNbEJHYlVOblRtbERSRTFwVEdwMVQyRnJSMWgzT0hkVVVHVTBUVkpxUVV0VE1rRnFSVUUxWkVOdGNVVjFPVnBwVG5wWlZIQkpNMlpCWlFwemJHdzRZMlZ6TXpseGFHVm5jMDh5WlV4NFdYSXJhM0JZYTFoUE5GcDRVMmt3UTNZcmNsSkZiV2xLWndvdExTMHRMVVZPUkNCRFJWSlVTVVpKUTBGVVJTMHRMUzB0Q2c9PSJ9fX19\",\"integratedTime\":1665481948,\"logIndex\":4884681,\"logID\":\"c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d\"}}",
        "dev.sigstore.cosign/certificate": "-----BEGIN CERTIFICATE-----\nMIICvTCCAkOgAwIBAgIUZqa88FvpduNj4e3+bNuWtsZQiIAwCgYIKoZIzj0EAwMw\nNzEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MR4wHAYDVQQDExVzaWdzdG9yZS1pbnRl\ncm1lZGlhdGUwHhcNMjIxMDExMDk1MjI3WhcNMjIxMDExMTAwMjI3WjAAMFkwEwYH\nKoZIzj0CAQYIKoZIzj0DAQcDQgAEk4xZ8kTx0MX0TEEIyAo78w9m01IsDQbV1OiK\nYW8NDYxHzrhYkN+k+gRWoHRw9kC5pvjIdn2hckl2pV1f4qivpaOCAWIwggFeMA4G\nA1UdDwEB/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAzAdBgNVHQ4EFgQUNZWY\nVXzCK/KUPlIpAa5dPtiKBVAwHwYDVR0jBBgwFoAU39Ppz1YkEZb5qNjpKFWixi4Y\nZD8wPwYDVR0RAQH/BDUwM4ExY29zaWduQGNvc2lnbi10ZXN0LTM2NTIwOS5pYW0u\nZ3NlcnZpY2VhY2NvdW50LmNvbTApBgorBgEEAYO/MAEBBBtodHRwczovL2FjY291\nbnRzLmdvb2dsZS5jb20wgYoGCisGAQQB1nkCBAIEfAR6AHgAdgAIYJLwKFL/aEXR\n0WsnhJxFZxisFj3DONJt5rwiBjZvcgAAAYPGdbkUAAAEAwBHMEUCIQCuVIwybmQu\nkTzI4zJplpRkoT43t5bwTFq4P1bhWNxp1AIgUyNxcOBt1AWors2RHtgJzJTH6ib6\nQ+PdUoG7rytgGGswCgYIKoZIzj0EAwMDaAAwZQIwUoUHg3dWENoUmXH79OYQ11ZC\np/jZd22PFmCgNiCDMiLjuOakGXw8wTPe4MRjAKS2AjEA5dCmqEu9ZiNzYTpI3fAe\nsll8ces39qhegsO2eLxYr+kpXkXO4ZxSi0Cv+rREmiJg\n-----END CERTIFICATE-----\n",
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
#     us-central1-docker.pkg.dev/$PROJECT_ID/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753

cosign verify-attestation \
  --key gcpkms://projects/$PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 \
    --policy policy.rego    \
      us-central1-docker.pkg.dev/$PROJECT_ID/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753  | jq '.'

will be validating against Rego policies: [policy.rego]

Verification for us-central1-docker.pkg.dev/cosign-test-365209/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753 --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - The signatures were verified against the specified public key
{
  "payloadType": "application/vnd.in-toto+json",
  "payload": "eyJfdHlwZSI6Imh0dHBzOi8vaW4tdG90by5pby9TdGF0ZW1lbnQvdjAuMSIsInByZWRpY2F0ZVR5cGUiOiJjb3NpZ24uc2lnc3RvcmUuZGV2L2F0dGVzdGF0aW9uL3YxIiwic3ViamVjdCI6W3sibmFtZSI6InVzLWNlbnRyYWwxLWRvY2tlci5wa2cuZGV2L2Nvc2lnbi10ZXN0LTM2NTIwOS9yZXBvMS9teWltYWdlIiwiZGlnZXN0Ijp7InNoYTI1NiI6IjgzYWIyYmE2Njg5NzEzZjJkNjgxMDRjZDIwOGZlYWRmZWJkZDZiYzg4MWM0NTVkY2I1NWQyYjQ1YWMzYTA3NTMifX1dLCJwcmVkaWNhdGUiOnsiRGF0YSI6InsgXCJwcm9qZWN0aWRcIjogXCJjb3NpZ24tdGVzdC0zNjUyMDlcIiwgXCJidWlsZGlkXCI6IFwiMDU5NGYyZTUtZjYwMC00OGRiLThhYTItZGFmNThlZTFhY2I1XCIsIFwiZm9vXCI6XCJiYXJcIiwgXCJjb21taXRzaGFcIjogXCJkZDkzZTllODkzZmZhZjJjNGNhZmViMmU1MzRiZjAzZjY2ZDdiZjI4XCIgfSIsIlRpbWVzdGFtcCI6IjIwMjItMTAtMTFUMDk6NTI6MjVaIn19",
  "signatures": [
    {
      "keyid": "",
      "sig": "MEUCIQDf4McOgck0jznjOKlzQGeX1Tayfmup0EC7wvkPhPEaEwIgSQCIl/6t2VAYRpvddDJru5CfIxVl5ArKmvxStQwVKDI="
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
      "name": "us-central1-docker.pkg.dev/cosign-test-365209/repo1/myimage",
      "digest": {
        "sha256": "83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753"
      }
    }
  ],
  "predicate": {
    "Data": "{ 
      \"projectid\": \"cosign-test-365209\", 
      \"buildid\": \"0594f2e5-f600-48db-8aa2-daf58ee1acb5\", 
      \"foo\":\"bar\", 
      \"commitsha\": \"dd93e9e893ffaf2c4cafeb2e534bf03f66d7bf28\" }",
    "Timestamp": "2022-10-11T09:52:25Z"
  }
}
```

Note the commit hash (`dd93e9e893ffaf2c4cafeb2e534bf03f66d7bf28`).  you can define a rego to validate that too

for the OIDC based signature,


```bash
COSIGN_EXPERIMENTAL=1 cosign verify-attestation  --policy policy.rego    \
        us-central1-docker.pkg.dev/$PROJECT_ID/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753  | jq '.'


Verification for us-central1-docker.pkg.dev/cosign-test-365209/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753 --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - Existence of the claims in the transparency log was verified offline
  - Any certificates were verified against the Fulcio roots.
Certificate subject:  cosign@cosign-test-365209.iam.gserviceaccount.com
Certificate issuer URL:  https://accounts.google.com
{
  "payloadType": "application/vnd.in-toto+json",
  "payload": "eyJfdHlwZSI6Imh0dHBzOi8vaW4tdG90by5pby9TdGF0ZW1lbnQvdjAuMSIsInByZWRpY2F0ZVR5cGUiOiJjb3NpZ24uc2lnc3RvcmUuZGV2L2F0dGVzdGF0aW9uL3YxIiwic3ViamVjdCI6W3sibmFtZSI6InVzLWNlbnRyYWwxLWRvY2tlci5wa2cuZGV2L2Nvc2lnbi10ZXN0LTM2NTIwOS9yZXBvMS9teWltYWdlIiwiZGlnZXN0Ijp7InNoYTI1NiI6IjgzYWIyYmE2Njg5NzEzZjJkNjgxMDRjZDIwOGZlYWRmZWJkZDZiYzg4MWM0NTVkY2I1NWQyYjQ1YWMzYTA3NTMifX1dLCJwcmVkaWNhdGUiOnsiRGF0YSI6InsgXCJwcm9qZWN0aWRcIjogXCJjb3NpZ24tdGVzdC0zNjUyMDlcIiwgXCJidWlsZGlkXCI6IFwiMDU5NGYyZTUtZjYwMC00OGRiLThhYTItZGFmNThlZTFhY2I1XCIsIFwiZm9vXCI6XCJiYXJcIiwgXCJjb21taXRzaGFcIjogXCJkZDkzZTllODkzZmZhZjJjNGNhZmViMmU1MzRiZjAzZjY2ZDdiZjI4XCIgfSIsIlRpbWVzdGFtcCI6IjIwMjItMTAtMTFUMDk6NTI6MzBaIn19",
  "signatures": [
    {
      "keyid": "",
      "sig": "MEYCIQDq3AWKVyXhMohD9V+ihEbJdK/9tS1d01M3ioRigAzT0wIhALvrmg6j7zA9Omq+kBkAyFLc2C2ENTr7pNhB3Pdb8v3h"
    }
  ]
}
```

### Verify with dockerhub image

I've also uploaded this sample to dockerhub so you can verify without container registry:

```bash
cosign sign --annotations=key1=value1 \
  --key gcpkms://projects/$PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 \
      docker.io/salrashid123/myimage:server

cosign verify --key cert/kms_pub.pem   \
    docker.io/salrashid123/myimage:server@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753  | jq '.'


COSIGN_EXPERIMENTAL=1 cosign attest \
  --key gcpkms://projects/$PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 \
  -f \
  --predicate=predicate.json  docker.io/salrashid123/myimage:server


COSIGN_EXPERIMENTAL=1 cosign verify-attestation  --key cert/kms_pub.pem --policy policy.rego    \
       docker.io/salrashid123/myimage:server@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753  | jq '.'
```

### Cosign and Rekor APIs

You can use the cosign and Rekor APIs as well.

The `client/main.go` sample application iterates over the signatures and attestations for the image hash in this repo.  


By default, it will scan the dockerhub registry but you can alter it to use your GCP Artifiact Registry.  


```bash
$ go run main.go 

>>>>>>>>>> Search rekor <<<<<<<<<<

## KMS
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


## email:cosign@cosign-test-363613.iam.gserviceaccount.com
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

## email:cosign@cosign-test-365209.iam.gserviceaccount.com
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

##  email:cosign@cosign-test-363615.iam.gserviceaccount.com
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
>>>>>>>>>> Verifying Image Signatures <<<<<<<<<<
Verified signature MEQCIGWaJFKrT1bS8FtK1avr8lUxCCK2f7DqMNFK0+SZrQSJAiB+hfhY73b99JFmM3H6kJaPSHVnEg50GCXskAzFHWX7nA==
  Image Ref {sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753}

```


### Sign without upload to registry


The following will sign an image with a key you provide
```bash
export IMAGE=docker.io/salrashid123/myimage:server
export IMAGE_DIGEST=$IMAGE@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753

gcloud kms keys versions get-public-key 1  \
   --key=key1 --keyring=cosignkr  \
    --location=global --output-file=/tmp/kms_pub.pem


### sign
$ cosign sign \
  --key gcpkms://projects/PROJECT_ID/locations/global/keyRings/cosignkr/cryptoKeys/key1/cryptoKeyVersions/1 \
  --upload=false $IMAGE_DIGEST --no-tlog-upload=true --output-signature sig.txt

$ cat sig.txt 
MEQCIG44Ca2Ngtvnj0bC7dvsPECgvXJJ88sVaTkOoMBR5HoqAiAXM8uUPWDMISYgv50mUSl7Oe2nFhP54Up/C4grKxVXPw==

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
  "payload": "eyJfdHlwZSI6Imh0dHBzOi8vaW4tdG90by5pby9TdGF0ZW1lbnQvdjAuMSIsInByZWRpY2F0ZVR5cGUiOiJjb3NpZ24uc2lnc3RvcmUuZGV2L2F0dGVzdGF0aW9uL3YxIiwic3ViamVjdCI6W3sibmFtZSI6ImluZGV4LmRvY2tlci5pby9zYWxyYXNoaWQxMjMvbXlpbWFnZSIsImRpZ2VzdCI6eyJzaGEyNTYiOiI4M2FiMmJhNjY4OTcxM2YyZDY4MTA0Y2QyMDhmZWFkZmViZGQ2YmM4ODFjNDU1ZGNiNTVkMmI0NWFjM2EwNzUzIn19XSwicHJlZGljYXRlIjp7IkRhdGEiOiJ7IFxuICAgIFwicHJvamVjdGlkXCI6IFwiJFBST0pFQ1RfSURcIiwgXG4gICAgXCJidWlsZGlkXCI6IFwiJEJVSUxEX0lEXCIsIFxuICAgIFwiZm9vXCI6IFwiYmFyXCIsIFxuICAgIFwiY29tbWl0c2hhXCI6IFwiZm9vXCJcbn0iLCJUaW1lc3RhbXAiOiIyMDIyLTEwLTE2VDAxOjQ0OjQxWiJ9fQ==",
  "signatures": [
    {
      "keyid": "",
      "sig": "MEQCIF+BmFeezMELIXiTSVanE6tuyaNz+Zc3hHgw6e9DXXQZAiAuQPZt4BUp4J9PDMUJbLWmB0ghLd7wvBWFqrjMjXfqmQ=="
    }
  ]
}
```

end to end


```bash
export IMAGE=us-central1-docker.pkg.dev/$PROJECT_ID/repo1/myimage:server
export IMAGE_DIGEST=us-central1-docker.pkg.dev/$PROJECT_ID/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753

### Create a signer
cosign generate-key-pair

mv cosign.key c1.key
mv cosign.pub c1.pub

cosign sign \
  --key c1.key \
  --upload=false $IMAGE_DIGEST --no-tlog-upload=true --output-signature sig.txt

cosign verify  --key c1.pub $IMAGE --signature sig.txt | jq '.'

$ cosign tree us-central1-docker.pkg.dev/$PROJECT_ID/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753
    📦 Supply Chain Security Related artifacts for an image: us-central1-docker.pkg.dev/PROJECT_ID/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753
    No Supply Chain Security Related Artifacts artifacts found for image us-central1-docker.pkg.dev/PROJECT_ID/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753

# attach as repo owner
cosign attach signature --signature `cat sig.txt` $IMAGE_DIGEST


$ cosign tree us-central1-docker.pkg.dev/$PROJECT_ID/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753
    📦 Supply Chain Security Related artifacts for an image: us-central1-docker.pkg.dev/PROJECT_ID/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753
    └── 🔐 Signatures for an image tag: us-central1-docker.pkg.dev/PROJECT_ID/repo1/myimage:sha256-83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753.sig
       └── 🍒 sha256:3795bcd1f5c026967d24d1250e20b5ffd76372cc0ac9690a31013cf04d0e5952

### Create a new signer

cosign generate-key-pair

mv cosign.key c2.key
mv cosign.pub c2.pub

cosign sign \
  --key c2.key \
  --upload=false $IMAGE_DIGEST --no-tlog-upload=true --output-signature sig.txt

cosign verify  --key c1.pub $IMAGE --signature sig.txt | jq '.'

# attach as repo owner
cosign attach signature --signature `cat sig.txt` $IMAGE_DIGEST

$ cosign tree us-central1-docker.pkg.dev/$PROJECT_ID/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753
    📦 Supply Chain Security Related artifacts for an image: us-central1-docker.pkg.dev/PROJECT_ID/repo1/myimage@sha256:83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753
    └── 🔐 Signatures for an image tag: us-central1-docker.pkg.dev/PROJECT_ID/repo1/myimage:sha256-83ab2ba6689713f2d68104cd208feadfebdd6bc881c455dcb55d2b45ac3a0753.sig
       ├── 🍒 sha256:3795bcd1f5c026967d24d1250e20b5ffd76372cc0ac9690a31013cf04d0e5952
       └── 🍒 sha256:3795bcd1f5c026967d24d1250e20b5ffd76372cc0ac9690a31013cf04d0e5952
```

---

thats as much as i know about this at the time of writing..

---


