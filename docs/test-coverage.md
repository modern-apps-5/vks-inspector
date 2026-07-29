
VKS Inspector project aims to quickly indentify common misconfigurations and errors in any vSphere with Tanzu environment.  The team is constantly incorporating field feedback to enhance the test coverage. Please open a Gitlab Issue with a FR to add a test that is missing.

## Test Coverage
  ### 
  - [x] DNS forward and reverse resolution should work for vCenter and NSX Manager.
  - [x] Ping/curl various network end points that they are reachable (DNS, NTP, VCenter, NSX Manager, <<Break these out separately>>)
  - [x] Validate vSphere API is accessible and provided credentials are valid.
  - [x] Validate existence of VDS specified in configuration YAML is valid.
  - [x] NTP drift between vCenter and ESXi hosts in Cluster is less than specified max(30 seconds).
  - [x] AVI Controller Login and SW Version
  - [x] AVI Service Engine Health Score above 75 for all Service Engines.
  - [x] AVI Virtual Service Health Score above 75 for all Virtual Services.
  - [x] AVI Load Balancer Pool Health Score above 75 for all Pools 
  - [x] 
  - [x]
  - [x] 
  - [x] 
  - [ ] 
  - [ ] 
  - [ ] 
  - [ ] 
  - [ ] 

  

