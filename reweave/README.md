# reweave

This is an effort to create a modern build environment for weave net, and simultaneously upgrade dependencies and the go compiler version.

This should solve some known problems, including:
* Incompatibility with some versions of CNI and containerd
* Vulnerabilities in the current weave images (2.8.1)
* Mismatch between the Docker Hub hosted weave:latest and weave:2.8.1 images
* Problems with multi-architecture images

The current focus of this effort is running weave net as a CNI plugin on Kubernetes. As such, other parts of weave net, such as the docker plugin, may receive less attention.

## Tasks

* Document the old build process
* Create a new build process
* Build, generate and test current images
* Upgrade Go compiler version
* Scan images for CVEs
* While there are fixable CVEs: 
    * Upgrade dependencies using CVE list as guideline
    * Build, generate new images, test
    * Scan new images for CVEs
