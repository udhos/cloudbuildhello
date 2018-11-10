# cloudbuildhello

gsutil mb gs://cloudbuildhello

gsutil ls gs://cloudbuildhello

gcloud builds submit --config cloudbuild.yaml .

## References

https://github.com/GoogleCloudPlatform/cloud-builders/tree/master/go
