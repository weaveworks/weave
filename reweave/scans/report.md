# Vulnerability Report

```
Report date: 2025-09-16
Unique vulnerability count: 52
Images version: 2.9.0
```

## Scanner Details

```
Application:         grype
Version:             0.100.0
BuildDate:           2025-09-15T21:51:57Z
GitCommit:           088112b26e638c139a513f387f7a6e51f1a8b76d
GitDescription:      v0.100.0
Platform:            linux/amd64
GoVersion:           go1.24.7
Compiler:            gc
Syft Version:        v1.33.0
Supported DB Schema: 6
```

## Vulnerabilities

### weave-kube: (51) 

```
NAME                              INSTALLED             FIXED IN                       TYPE       VULNERABILITY        SEVERITY  EPSS           RISK   
curl                              8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0665        Critical  4.0% (87th)    3.8    
libcurl                           8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0665        Critical  4.0% (87th)    3.8    
libxml2                           2.12.7-r0                                            apk        CVE-2024-40896       Critical  0.4% (58th)    0.3    
curl                              8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0725        High      0.4% (61st)    0.3    
libcurl                           8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0725        High      0.4% (61st)    0.3    
bind-libs                         9.18.32-r0            9.18.33-r0                     apk        CVE-2024-11187       High      0.4% (57th)    0.3    
bind-tools                        9.18.32-r0            9.18.33-r0                     apk        CVE-2024-11187       High      0.4% (57th)    0.3    
bind-libs                         9.18.32-r0            9.18.33-r0                     apk        CVE-2024-12705       High      0.3% (49th)    0.2    
bind-tools                        9.18.32-r0            9.18.33-r0                     apk        CVE-2024-12705       High      0.3% (49th)    0.2    
libcrypto3                        3.3.2-r1              3.3.3-r0                       apk        CVE-2024-12797       Medium    0.3% (49th)    0.1    
libssl3                           3.3.2-r1              3.3.3-r0                       apk        CVE-2024-12797       Medium    0.3% (49th)    0.1    
xz-libs                           5.6.2-r0              5.6.2-r1                       apk        CVE-2025-31115       High      0.2% (39th)    0.1    
golang.org/x/crypto               v0.31.0               0.35.0                         go-module  GHSA-hcg3-q754-cr77  High      0.2% (40th)    0.1    
c-ares                            1.33.1-r0                                            apk        CVE-2025-31498       High      0.1% (29th)    < 0.1  
golang.org/x/oauth2               v0.10.0               0.27.0                         go-module  GHSA-6v2p-p543-phr9  High      < 0.1% (23rd)  < 0.1  
libxml2                           2.12.7-r0             2.12.7-r2                      apk        CVE-2025-27113       High      < 0.1% (18th)  < 0.1  
stdlib                            go1.23.1              *1.23.12, 1.24.6               go-module  CVE-2025-47907       High      < 0.1% (18th)  < 0.1  
libcrypto3                        3.3.2-r1              3.3.2-r2                       apk        CVE-2024-13176       Medium    < 0.1% (24th)  < 0.1  
libssl3                           3.3.2-r1              3.3.2-r2                       apk        CVE-2024-13176       Medium    < 0.1% (24th)  < 0.1  
curl                              8.11.1-r0                                            apk        CVE-2025-9086        High      < 0.1% (10th)  < 0.1  
stdlib                            go1.23.1              1.22.11, *1.23.5, 1.24.0-rc.2  go-module  CVE-2024-45336       Medium    < 0.1% (14th)  < 0.1  
curl                              8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0167        Low       < 0.1% (23rd)  < 0.1  
libcurl                           8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0167        Low       < 0.1% (23rd)  < 0.1  
libxml2                           2.12.7-r0             2.12.10-r0                     apk        CVE-2025-32414       High      < 0.1% (7th)   < 0.1  
stdlib                            go1.23.1              1.22.11, *1.23.5, 1.24.0-rc.2  go-module  CVE-2024-45341       Medium    < 0.1% (9th)   < 0.1  
libxml2                           2.12.7-r0             2.12.10-r0                     apk        CVE-2025-32415       High      < 0.1% (5th)   < 0.1  
curl                              8.11.1-r0                                            apk        CVE-2025-10148       Medium    < 0.1% (5th)   < 0.1  
bind-libs                         9.18.32-r0            9.18.37-r0                     apk        CVE-2025-40775       High      < 0.1% (2nd)   < 0.1  
bind-tools                        9.18.32-r0            9.18.37-r0                     apk        CVE-2025-40775       High      < 0.1% (2nd)   < 0.1  
stdlib                            go1.23.1              *1.23.8, 1.24.2                go-module  CVE-2025-22871       Critical  < 0.1% (1st)   < 0.1  
stdlib                            go1.23.1              *1.23.10, 1.24.4               go-module  CVE-2025-4673        Medium    < 0.1% (3rd)   < 0.1  
golang.org/x/net                  v0.33.0               0.38.0                         go-module  GHSA-vvgc-356p-c3xw  Medium    < 0.1% (4th)   < 0.1  
curl                              8.11.1-r0                                            apk        CVE-2025-4947        Medium    < 0.1% (3rd)   < 0.1  
musl                              1.2.5-r0              1.2.5-r1                       apk        CVE-2025-26519       High      < 0.1% (1st)   < 0.1  
musl-utils                        1.2.5-r0              1.2.5-r1                       apk        CVE-2025-26519       High      < 0.1% (1st)   < 0.1  
curl                              8.11.1-r0                                            apk        CVE-2025-5025        Medium    < 0.1% (3rd)   < 0.1  
busybox                           1.36.1-r29                                           apk        CVE-2024-58251       Low       < 0.1% (7th)   < 0.1  
busybox-binsh                     1.36.1-r29                                           apk        CVE-2024-58251       Low       < 0.1% (7th)   < 0.1  
ssl_client                        1.36.1-r29                                           apk        CVE-2024-58251       Low       < 0.1% (7th)   < 0.1  
bind-libs                         9.18.32-r0                                           apk        CVE-2025-40776       High      < 0.1% (0th)   < 0.1  
bind-tools                        9.18.32-r0                                           apk        CVE-2025-40776       High      < 0.1% (0th)   < 0.1  
golang.org/x/net                  v0.33.0               0.36.0                         go-module  GHSA-qxp5-gwg8-xv66  Medium    < 0.1% (2nd)   < 0.1  
libxml2                           2.12.7-r0             2.12.7-r1                      apk        CVE-2024-56171       High      < 0.1% (0th)   < 0.1  
libxml2                           2.12.7-r0             2.12.7-r1                      apk        CVE-2025-24928       High      < 0.1% (0th)   < 0.1  
busybox                           1.36.1-r29                                           apk        CVE-2025-46394       Low       < 0.1% (4th)   < 0.1  
busybox-binsh                     1.36.1-r29                                           apk        CVE-2025-46394       Low       < 0.1% (4th)   < 0.1  
ssl_client                        1.36.1-r29                                           apk        CVE-2025-46394       Low       < 0.1% (4th)   < 0.1  
github.com/containerd/containerd  v1.7.11               1.7.27                         go-module  GHSA-265r-hfxg-fhmg  Medium    < 0.1% (1st)   < 0.1  
stdlib                            go1.23.1              1.22.12, *1.23.6, 1.24.0-rc.3  go-module  CVE-2025-22866       Medium    < 0.1% (1st)   < 0.1  
stdlib                            go1.23.1              *1.23.11, 1.24.5               go-module  CVE-2025-4674        High      < 0.1% (0th)   < 0.1  
github.com/docker/docker          v25.0.6+incompatible                                 go-module  GHSA-4vq8-7jfc-9cvp  Low       < 0.1% (1st)   < 0.1
```

### weave-npc: (23) 

```
NAME                 INSTALLED   FIXED IN                       TYPE       VULNERABILITY        SEVERITY  EPSS           RISK   
libcrypto3           3.3.2-r1    3.3.3-r0                       apk        CVE-2024-12797       Medium    0.3% (49th)    0.1    
libssl3              3.3.2-r1    3.3.3-r0                       apk        CVE-2024-12797       Medium    0.3% (49th)    0.1    
golang.org/x/crypto  v0.31.0     0.35.0                         go-module  GHSA-hcg3-q754-cr77  High      0.2% (40th)    0.1    
golang.org/x/oauth2  v0.10.0     0.27.0                         go-module  GHSA-6v2p-p543-phr9  High      < 0.1% (23rd)  < 0.1  
stdlib               go1.23.1    *1.23.12, 1.24.6               go-module  CVE-2025-47907       High      < 0.1% (18th)  < 0.1  
libcrypto3           3.3.2-r1    3.3.2-r2                       apk        CVE-2024-13176       Medium    < 0.1% (24th)  < 0.1  
libssl3              3.3.2-r1    3.3.2-r2                       apk        CVE-2024-13176       Medium    < 0.1% (24th)  < 0.1  
stdlib               go1.23.1    1.22.11, *1.23.5, 1.24.0-rc.2  go-module  CVE-2024-45336       Medium    < 0.1% (14th)  < 0.1  
stdlib               go1.23.1    1.22.11, *1.23.5, 1.24.0-rc.2  go-module  CVE-2024-45341       Medium    < 0.1% (9th)   < 0.1  
stdlib               go1.23.1    *1.23.8, 1.24.2                go-module  CVE-2025-22871       Critical  < 0.1% (1st)   < 0.1  
stdlib               go1.23.1    *1.23.10, 1.24.4               go-module  CVE-2025-4673        Medium    < 0.1% (3rd)   < 0.1  
golang.org/x/net     v0.33.0     0.38.0                         go-module  GHSA-vvgc-356p-c3xw  Medium    < 0.1% (4th)   < 0.1  
musl                 1.2.5-r0    1.2.5-r1                       apk        CVE-2025-26519       High      < 0.1% (1st)   < 0.1  
musl-utils           1.2.5-r0    1.2.5-r1                       apk        CVE-2025-26519       High      < 0.1% (1st)   < 0.1  
busybox              1.36.1-r29                                 apk        CVE-2024-58251       Low       < 0.1% (7th)   < 0.1  
busybox-binsh        1.36.1-r29                                 apk        CVE-2024-58251       Low       < 0.1% (7th)   < 0.1  
ssl_client           1.36.1-r29                                 apk        CVE-2024-58251       Low       < 0.1% (7th)   < 0.1  
golang.org/x/net     v0.33.0     0.36.0                         go-module  GHSA-qxp5-gwg8-xv66  Medium    < 0.1% (2nd)   < 0.1  
busybox              1.36.1-r29                                 apk        CVE-2025-46394       Low       < 0.1% (4th)   < 0.1  
busybox-binsh        1.36.1-r29                                 apk        CVE-2025-46394       Low       < 0.1% (4th)   < 0.1  
ssl_client           1.36.1-r29                                 apk        CVE-2025-46394       Low       < 0.1% (4th)   < 0.1  
stdlib               go1.23.1    1.22.12, *1.23.6, 1.24.0-rc.3  go-module  CVE-2025-22866       Medium    < 0.1% (1st)   < 0.1  
stdlib               go1.23.1    *1.23.11, 1.24.5               go-module  CVE-2025-4674        High      < 0.1% (0th)   < 0.1
```

### weave: (50) 

```
NAME                              INSTALLED             FIXED IN                       TYPE       VULNERABILITY        SEVERITY  EPSS           RISK   
curl                              8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0665        Critical  4.0% (87th)    3.8    
libcurl                           8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0665        Critical  4.0% (87th)    3.8    
libxml2                           2.12.7-r0                                            apk        CVE-2024-40896       Critical  0.4% (58th)    0.3    
curl                              8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0725        High      0.4% (61st)    0.3    
libcurl                           8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0725        High      0.4% (61st)    0.3    
bind-libs                         9.18.32-r0            9.18.33-r0                     apk        CVE-2024-11187       High      0.4% (57th)    0.3    
bind-tools                        9.18.32-r0            9.18.33-r0                     apk        CVE-2024-11187       High      0.4% (57th)    0.3    
bind-libs                         9.18.32-r0            9.18.33-r0                     apk        CVE-2024-12705       High      0.3% (49th)    0.2    
bind-tools                        9.18.32-r0            9.18.33-r0                     apk        CVE-2024-12705       High      0.3% (49th)    0.2    
libcrypto3                        3.3.2-r1              3.3.3-r0                       apk        CVE-2024-12797       Medium    0.3% (49th)    0.1    
libssl3                           3.3.2-r1              3.3.3-r0                       apk        CVE-2024-12797       Medium    0.3% (49th)    0.1    
xz-libs                           5.6.2-r0              5.6.2-r1                       apk        CVE-2025-31115       High      0.2% (39th)    0.1    
golang.org/x/crypto               v0.31.0               0.35.0                         go-module  GHSA-hcg3-q754-cr77  High      0.2% (40th)    0.1    
c-ares                            1.33.1-r0                                            apk        CVE-2025-31498       High      0.1% (29th)    < 0.1  
libxml2                           2.12.7-r0             2.12.7-r2                      apk        CVE-2025-27113       High      < 0.1% (18th)  < 0.1  
stdlib                            go1.23.1              *1.23.12, 1.24.6               go-module  CVE-2025-47907       High      < 0.1% (18th)  < 0.1  
libcrypto3                        3.3.2-r1              3.3.2-r2                       apk        CVE-2024-13176       Medium    < 0.1% (24th)  < 0.1  
libssl3                           3.3.2-r1              3.3.2-r2                       apk        CVE-2024-13176       Medium    < 0.1% (24th)  < 0.1  
curl                              8.11.1-r0                                            apk        CVE-2025-9086        High      < 0.1% (10th)  < 0.1  
stdlib                            go1.23.1              1.22.11, *1.23.5, 1.24.0-rc.2  go-module  CVE-2024-45336       Medium    < 0.1% (14th)  < 0.1  
curl                              8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0167        Low       < 0.1% (23rd)  < 0.1  
libcurl                           8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0167        Low       < 0.1% (23rd)  < 0.1  
libxml2                           2.12.7-r0             2.12.10-r0                     apk        CVE-2025-32414       High      < 0.1% (7th)   < 0.1  
stdlib                            go1.23.1              1.22.11, *1.23.5, 1.24.0-rc.2  go-module  CVE-2024-45341       Medium    < 0.1% (9th)   < 0.1  
libxml2                           2.12.7-r0             2.12.10-r0                     apk        CVE-2025-32415       High      < 0.1% (5th)   < 0.1  
curl                              8.11.1-r0                                            apk        CVE-2025-10148       Medium    < 0.1% (5th)   < 0.1  
bind-libs                         9.18.32-r0            9.18.37-r0                     apk        CVE-2025-40775       High      < 0.1% (2nd)   < 0.1  
bind-tools                        9.18.32-r0            9.18.37-r0                     apk        CVE-2025-40775       High      < 0.1% (2nd)   < 0.1  
stdlib                            go1.23.1              *1.23.8, 1.24.2                go-module  CVE-2025-22871       Critical  < 0.1% (1st)   < 0.1  
stdlib                            go1.23.1              *1.23.10, 1.24.4               go-module  CVE-2025-4673        Medium    < 0.1% (3rd)   < 0.1  
golang.org/x/net                  v0.33.0               0.38.0                         go-module  GHSA-vvgc-356p-c3xw  Medium    < 0.1% (4th)   < 0.1  
curl                              8.11.1-r0                                            apk        CVE-2025-4947        Medium    < 0.1% (3rd)   < 0.1  
musl                              1.2.5-r0              1.2.5-r1                       apk        CVE-2025-26519       High      < 0.1% (1st)   < 0.1  
musl-utils                        1.2.5-r0              1.2.5-r1                       apk        CVE-2025-26519       High      < 0.1% (1st)   < 0.1  
curl                              8.11.1-r0                                            apk        CVE-2025-5025        Medium    < 0.1% (3rd)   < 0.1  
busybox                           1.36.1-r29                                           apk        CVE-2024-58251       Low       < 0.1% (7th)   < 0.1  
busybox-binsh                     1.36.1-r29                                           apk        CVE-2024-58251       Low       < 0.1% (7th)   < 0.1  
ssl_client                        1.36.1-r29                                           apk        CVE-2024-58251       Low       < 0.1% (7th)   < 0.1  
bind-libs                         9.18.32-r0                                           apk        CVE-2025-40776       High      < 0.1% (0th)   < 0.1  
bind-tools                        9.18.32-r0                                           apk        CVE-2025-40776       High      < 0.1% (0th)   < 0.1  
golang.org/x/net                  v0.33.0               0.36.0                         go-module  GHSA-qxp5-gwg8-xv66  Medium    < 0.1% (2nd)   < 0.1  
libxml2                           2.12.7-r0             2.12.7-r1                      apk        CVE-2024-56171       High      < 0.1% (0th)   < 0.1  
libxml2                           2.12.7-r0             2.12.7-r1                      apk        CVE-2025-24928       High      < 0.1% (0th)   < 0.1  
busybox                           1.36.1-r29                                           apk        CVE-2025-46394       Low       < 0.1% (4th)   < 0.1  
busybox-binsh                     1.36.1-r29                                           apk        CVE-2025-46394       Low       < 0.1% (4th)   < 0.1  
ssl_client                        1.36.1-r29                                           apk        CVE-2025-46394       Low       < 0.1% (4th)   < 0.1  
github.com/containerd/containerd  v1.7.11               1.7.27                         go-module  GHSA-265r-hfxg-fhmg  Medium    < 0.1% (1st)   < 0.1  
stdlib                            go1.23.1              1.22.12, *1.23.6, 1.24.0-rc.3  go-module  CVE-2025-22866       Medium    < 0.1% (1st)   < 0.1  
stdlib                            go1.23.1              *1.23.11, 1.24.5               go-module  CVE-2025-4674        High      < 0.1% (0th)   < 0.1  
github.com/docker/docker          v25.0.6+incompatible                                 go-module  GHSA-4vq8-7jfc-9cvp  Low       < 0.1% (1st)   < 0.1
```

### weaveexec: (50) 

```
NAME                              INSTALLED             FIXED IN                       TYPE       VULNERABILITY        SEVERITY  EPSS           RISK   
curl                              8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0665        Critical  4.0% (87th)    3.8    
libcurl                           8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0665        Critical  4.0% (87th)    3.8    
libxml2                           2.12.7-r0                                            apk        CVE-2024-40896       Critical  0.4% (58th)    0.3    
curl                              8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0725        High      0.4% (61st)    0.3    
libcurl                           8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0725        High      0.4% (61st)    0.3    
bind-libs                         9.18.32-r0            9.18.33-r0                     apk        CVE-2024-11187       High      0.4% (57th)    0.3    
bind-tools                        9.18.32-r0            9.18.33-r0                     apk        CVE-2024-11187       High      0.4% (57th)    0.3    
bind-libs                         9.18.32-r0            9.18.33-r0                     apk        CVE-2024-12705       High      0.3% (49th)    0.2    
bind-tools                        9.18.32-r0            9.18.33-r0                     apk        CVE-2024-12705       High      0.3% (49th)    0.2    
libcrypto3                        3.3.2-r1              3.3.3-r0                       apk        CVE-2024-12797       Medium    0.3% (49th)    0.1    
libssl3                           3.3.2-r1              3.3.3-r0                       apk        CVE-2024-12797       Medium    0.3% (49th)    0.1    
xz-libs                           5.6.2-r0              5.6.2-r1                       apk        CVE-2025-31115       High      0.2% (39th)    0.1    
golang.org/x/crypto               v0.31.0               0.35.0                         go-module  GHSA-hcg3-q754-cr77  High      0.2% (40th)    0.1    
c-ares                            1.33.1-r0                                            apk        CVE-2025-31498       High      0.1% (29th)    < 0.1  
libxml2                           2.12.7-r0             2.12.7-r2                      apk        CVE-2025-27113       High      < 0.1% (18th)  < 0.1  
stdlib                            go1.23.1              *1.23.12, 1.24.6               go-module  CVE-2025-47907       High      < 0.1% (18th)  < 0.1  
libcrypto3                        3.3.2-r1              3.3.2-r2                       apk        CVE-2024-13176       Medium    < 0.1% (24th)  < 0.1  
libssl3                           3.3.2-r1              3.3.2-r2                       apk        CVE-2024-13176       Medium    < 0.1% (24th)  < 0.1  
curl                              8.11.1-r0                                            apk        CVE-2025-9086        High      < 0.1% (10th)  < 0.1  
stdlib                            go1.23.1              1.22.11, *1.23.5, 1.24.0-rc.2  go-module  CVE-2024-45336       Medium    < 0.1% (14th)  < 0.1  
curl                              8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0167        Low       < 0.1% (23rd)  < 0.1  
libcurl                           8.11.1-r0             8.12.0-r0                      apk        CVE-2025-0167        Low       < 0.1% (23rd)  < 0.1  
libxml2                           2.12.7-r0             2.12.10-r0                     apk        CVE-2025-32414       High      < 0.1% (7th)   < 0.1  
stdlib                            go1.23.1              1.22.11, *1.23.5, 1.24.0-rc.2  go-module  CVE-2024-45341       Medium    < 0.1% (9th)   < 0.1  
libxml2                           2.12.7-r0             2.12.10-r0                     apk        CVE-2025-32415       High      < 0.1% (5th)   < 0.1  
curl                              8.11.1-r0                                            apk        CVE-2025-10148       Medium    < 0.1% (5th)   < 0.1  
bind-libs                         9.18.32-r0            9.18.37-r0                     apk        CVE-2025-40775       High      < 0.1% (2nd)   < 0.1  
bind-tools                        9.18.32-r0            9.18.37-r0                     apk        CVE-2025-40775       High      < 0.1% (2nd)   < 0.1  
stdlib                            go1.23.1              *1.23.8, 1.24.2                go-module  CVE-2025-22871       Critical  < 0.1% (1st)   < 0.1  
stdlib                            go1.23.1              *1.23.10, 1.24.4               go-module  CVE-2025-4673        Medium    < 0.1% (3rd)   < 0.1  
golang.org/x/net                  v0.33.0               0.38.0                         go-module  GHSA-vvgc-356p-c3xw  Medium    < 0.1% (4th)   < 0.1  
curl                              8.11.1-r0                                            apk        CVE-2025-4947        Medium    < 0.1% (3rd)   < 0.1  
musl                              1.2.5-r0              1.2.5-r1                       apk        CVE-2025-26519       High      < 0.1% (1st)   < 0.1  
musl-utils                        1.2.5-r0              1.2.5-r1                       apk        CVE-2025-26519       High      < 0.1% (1st)   < 0.1  
curl                              8.11.1-r0                                            apk        CVE-2025-5025        Medium    < 0.1% (3rd)   < 0.1  
busybox                           1.36.1-r29                                           apk        CVE-2024-58251       Low       < 0.1% (7th)   < 0.1  
busybox-binsh                     1.36.1-r29                                           apk        CVE-2024-58251       Low       < 0.1% (7th)   < 0.1  
ssl_client                        1.36.1-r29                                           apk        CVE-2024-58251       Low       < 0.1% (7th)   < 0.1  
bind-libs                         9.18.32-r0                                           apk        CVE-2025-40776       High      < 0.1% (0th)   < 0.1  
bind-tools                        9.18.32-r0                                           apk        CVE-2025-40776       High      < 0.1% (0th)   < 0.1  
golang.org/x/net                  v0.33.0               0.36.0                         go-module  GHSA-qxp5-gwg8-xv66  Medium    < 0.1% (2nd)   < 0.1  
libxml2                           2.12.7-r0             2.12.7-r1                      apk        CVE-2024-56171       High      < 0.1% (0th)   < 0.1  
libxml2                           2.12.7-r0             2.12.7-r1                      apk        CVE-2025-24928       High      < 0.1% (0th)   < 0.1  
busybox                           1.36.1-r29                                           apk        CVE-2025-46394       Low       < 0.1% (4th)   < 0.1  
busybox-binsh                     1.36.1-r29                                           apk        CVE-2025-46394       Low       < 0.1% (4th)   < 0.1  
ssl_client                        1.36.1-r29                                           apk        CVE-2025-46394       Low       < 0.1% (4th)   < 0.1  
github.com/containerd/containerd  v1.7.11               1.7.27                         go-module  GHSA-265r-hfxg-fhmg  Medium    < 0.1% (1st)   < 0.1  
stdlib                            go1.23.1              1.22.12, *1.23.6, 1.24.0-rc.3  go-module  CVE-2025-22866       Medium    < 0.1% (1st)   < 0.1  
stdlib                            go1.23.1              *1.23.11, 1.24.5               go-module  CVE-2025-4674        High      < 0.1% (0th)   < 0.1  
github.com/docker/docker          v25.0.6+incompatible                                 go-module  GHSA-4vq8-7jfc-9cvp  Low       < 0.1% (1st)   < 0.1
```

