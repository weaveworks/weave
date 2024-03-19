# Vulnerability Report

```
Report date: 2024-03-19
Unique vulnerability count: 39
Images version: 2.8.4-beta2
```

## Scanner Details

```
Application:         grype
Version:             0.74.7
BuildDate:           2024-02-26T18:24:14Z
GitCommit:           987238519b8d6e302130ab715f20daed6634da68
GitDescription:      v0.74.7
Platform:            linux/amd64
GoVersion:           go1.21.7
Compiler:            gc
Syft Version:        v0.105.1
Supported DB Schema: 5
```

## Vulnerabilities

### weave-kube: (20) 

```
NAME                        INSTALLED   FIXED-IN  TYPE       VULNERABILITY        SEVERITY 
busybox                     1.36.1-r15            apk        CVE-2023-42366       Medium    
busybox                     1.36.1-r15            apk        CVE-2023-42365       Medium    
busybox                     1.36.1-r15            apk        CVE-2023-42364       Medium    
busybox                     1.36.1-r15            apk        CVE-2023-42363       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42366       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42365       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42364       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42363       Medium    
curl                        8.5.0-r0              apk        CVE-2024-0853        Medium    
google.golang.org/protobuf  v1.31.0     1.33.0    go-module  GHSA-8r3f-844c-mc37  Medium    
libuv                       1.47.0-r0             apk        CVE-2024-24806       High      
ssl_client                  1.36.1-r15            apk        CVE-2023-42366       Medium    
ssl_client                  1.36.1-r15            apk        CVE-2023-42365       Medium    
ssl_client                  1.36.1-r15            apk        CVE-2023-42364       Medium    
ssl_client                  1.36.1-r15            apk        CVE-2023-42363       Medium    
stdlib                      go1.21.6              go-module  CVE-2024-24785       Unknown   
stdlib                      go1.21.6              go-module  CVE-2024-24784       Unknown   
stdlib                      go1.21.6              go-module  CVE-2024-24783       Unknown   
stdlib                      go1.21.6              go-module  CVE-2023-45290       Unknown   
stdlib                      go1.21.6              go-module  CVE-2023-45289       Unknown
```

### weave-npc: (18) 

```
NAME                        INSTALLED   FIXED-IN  TYPE       VULNERABILITY        SEVERITY 
busybox                     1.36.1-r15            apk        CVE-2023-42366       Medium    
busybox                     1.36.1-r15            apk        CVE-2023-42365       Medium    
busybox                     1.36.1-r15            apk        CVE-2023-42364       Medium    
busybox                     1.36.1-r15            apk        CVE-2023-42363       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42366       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42365       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42364       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42363       Medium    
google.golang.org/protobuf  v1.31.0     1.33.0    go-module  GHSA-8r3f-844c-mc37  Medium    
ssl_client                  1.36.1-r15            apk        CVE-2023-42366       Medium    
ssl_client                  1.36.1-r15            apk        CVE-2023-42365       Medium    
ssl_client                  1.36.1-r15            apk        CVE-2023-42364       Medium    
ssl_client                  1.36.1-r15            apk        CVE-2023-42363       Medium    
stdlib                      go1.21.6              go-module  CVE-2024-24785       Unknown   
stdlib                      go1.21.6              go-module  CVE-2024-24784       Unknown   
stdlib                      go1.21.6              go-module  CVE-2024-24783       Unknown   
stdlib                      go1.21.6              go-module  CVE-2023-45290       Unknown   
stdlib                      go1.21.6              go-module  CVE-2023-45289       Unknown
```

### weave: (20) 

```
NAME                        INSTALLED   FIXED-IN  TYPE       VULNERABILITY        SEVERITY 
busybox                     1.36.1-r15            apk        CVE-2023-42366       Medium    
busybox                     1.36.1-r15            apk        CVE-2023-42365       Medium    
busybox                     1.36.1-r15            apk        CVE-2023-42364       Medium    
busybox                     1.36.1-r15            apk        CVE-2023-42363       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42366       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42365       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42364       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42363       Medium    
curl                        8.5.0-r0              apk        CVE-2024-0853        Medium    
google.golang.org/protobuf  v1.31.0     1.33.0    go-module  GHSA-8r3f-844c-mc37  Medium    
libuv                       1.47.0-r0             apk        CVE-2024-24806       High      
ssl_client                  1.36.1-r15            apk        CVE-2023-42366       Medium    
ssl_client                  1.36.1-r15            apk        CVE-2023-42365       Medium    
ssl_client                  1.36.1-r15            apk        CVE-2023-42364       Medium    
ssl_client                  1.36.1-r15            apk        CVE-2023-42363       Medium    
stdlib                      go1.21.6              go-module  CVE-2024-24785       Unknown   
stdlib                      go1.21.6              go-module  CVE-2024-24784       Unknown   
stdlib                      go1.21.6              go-module  CVE-2024-24783       Unknown   
stdlib                      go1.21.6              go-module  CVE-2023-45290       Unknown   
stdlib                      go1.21.6              go-module  CVE-2023-45289       Unknown
```

### weaveexec: (20) 

```
NAME                        INSTALLED   FIXED-IN  TYPE       VULNERABILITY        SEVERITY 
busybox                     1.36.1-r15            apk        CVE-2023-42366       Medium    
busybox                     1.36.1-r15            apk        CVE-2023-42365       Medium    
busybox                     1.36.1-r15            apk        CVE-2023-42364       Medium    
busybox                     1.36.1-r15            apk        CVE-2023-42363       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42366       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42365       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42364       Medium    
busybox-binsh               1.36.1-r15            apk        CVE-2023-42363       Medium    
curl                        8.5.0-r0              apk        CVE-2024-0853        Medium    
google.golang.org/protobuf  v1.31.0     1.33.0    go-module  GHSA-8r3f-844c-mc37  Medium    
libuv                       1.47.0-r0             apk        CVE-2024-24806       High      
ssl_client                  1.36.1-r15            apk        CVE-2023-42366       Medium    
ssl_client                  1.36.1-r15            apk        CVE-2023-42365       Medium    
ssl_client                  1.36.1-r15            apk        CVE-2023-42364       Medium    
ssl_client                  1.36.1-r15            apk        CVE-2023-42363       Medium    
stdlib                      go1.21.6              go-module  CVE-2024-24785       Unknown   
stdlib                      go1.21.6              go-module  CVE-2024-24784       Unknown   
stdlib                      go1.21.6              go-module  CVE-2024-24783       Unknown   
stdlib                      go1.21.6              go-module  CVE-2023-45290       Unknown   
stdlib                      go1.21.6              go-module  CVE-2023-45289       Unknown
```

### weavedb: (0) 

```
No vulnerabilities found
```

### network-tester: (19) 

```
NAME           INSTALLED   FIXED-IN  TYPE       VULNERABILITY   SEVERITY 
busybox        1.36.1-r15            apk        CVE-2023-42366  Medium    
busybox        1.36.1-r15            apk        CVE-2023-42365  Medium    
busybox        1.36.1-r15            apk        CVE-2023-42364  Medium    
busybox        1.36.1-r15            apk        CVE-2023-42363  Medium    
busybox-binsh  1.36.1-r15            apk        CVE-2023-42366  Medium    
busybox-binsh  1.36.1-r15            apk        CVE-2023-42365  Medium    
busybox-binsh  1.36.1-r15            apk        CVE-2023-42364  Medium    
busybox-binsh  1.36.1-r15            apk        CVE-2023-42363  Medium    
curl           8.5.0-r0              apk        CVE-2024-0853   Medium    
libuv          1.47.0-r0             apk        CVE-2024-24806  High      
ssl_client     1.36.1-r15            apk        CVE-2023-42366  Medium    
ssl_client     1.36.1-r15            apk        CVE-2023-42365  Medium    
ssl_client     1.36.1-r15            apk        CVE-2023-42364  Medium    
ssl_client     1.36.1-r15            apk        CVE-2023-42363  Medium    
stdlib         go1.21.6              go-module  CVE-2024-24785  Unknown   
stdlib         go1.21.6              go-module  CVE-2024-24784  Unknown   
stdlib         go1.21.6              go-module  CVE-2024-24783  Unknown   
stdlib         go1.21.6              go-module  CVE-2023-45290  Unknown   
stdlib         go1.21.6              go-module  CVE-2023-45289  Unknown
```

