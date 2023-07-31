# reweave

This is an effort to create a modern build environment for weave net, and simultaneously upgrade dependencies and the go compiler version.

This should solve some known problems, including:
* Incompatibility with some versions of CNI and containerd
* Vulnerabilities in the current weave images (2.8.1)
* Mismatch between the Docker Hub hosted weave:latest and weave:2.8.1 images
* Problems with multi-architecture images

The current focus of this effort is running weave net as a CNI plugin on Kubernetes. As such, other parts of weave net, such as the docker plugin, may receive less attention.

## Tasks

* ~~Document the old build process~~Old build process documented in [BUILDING-OLD.md](BUILDING-OLD.md)
* ~~Create a image scanning process and scan current image~~Image scanning process created, scan reports generated for 2.8.1
* ~~Create a new build process~~New build process created, documented in [BUILDING.md](BUILDING.md)
* ~~Build, generate and test current images~~Built and scanned v2.8.2-beta*
* ~~Upgrade Go compiler version~~Go version upgraded to 1.20
* Scan images for CVEs
* While there are fixable CVEs: 
    * Upgrade dependencies using CVE list as guideline
    * Build, generate new images, test
    * Scan new images for CVEs
After a few rounds of the above, several components have been upgraded (details in the [CHANGELOG](CHANGELOG.md)). This process will continue.