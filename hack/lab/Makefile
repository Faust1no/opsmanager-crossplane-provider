# ============================================================
#  Makefile — Local Dev Stack Bootstrap
#  Installs: kind cluster | MongoDB Controllers for K8s v1.7.0
#            Ops Manager | Crossplane | Headlamp | MinIO
#
#  Operator image : quay.io/mongodb/mongodb-kubernetes:1.7.0
#  Helm chart     : mongodb/mongodb-kubernetes
#  Source         : https://github.com/mongodb/mongodb-kubernetes
# ============================================================

SHELL := /bin/bash

# ── Configuration ────────────────────────────────────────────
CLUSTER_NAME        ?= local-dev
KIND_CONFIG         ?= kind-config.yaml
KUBECONFIG          ?= $(HOME)/.kube/config

MCK_VERSION         := 1.7.0
MCK_NAMESPACE       := mongodb
# mongodb/mongodb-kubernetes chart (deploys the unified Controllers for K8s)
MONGODB_HELM_REPO   := https://mongodb.github.io/helm-charts

OPS_MANAGER_VERSION := 7.0.11
OPS_MANAGER_APPDB   := 6.0.14-ubi8

CROSSPLANE_NAMESPACE := crossplane-system
CROSSPLANE_VERSION   := 1.15.0

HEADLAMP_NAMESPACE   := headlamp

MINIO_NAMESPACE      := minio
MINIO_ROOT_USER      := minioadmin
MINIO_ROOT_PASSWORD  := minioadmin123
MINIO_BUCKET         := ops-manager-blockstore

GLAUTH_NAMESPACE     := glauth

# ── Test MongoDB Cluster ──────────────────────────────────────
# These values must be created manually in Ops Manager (Programmatic API Key
# with ORG_OWNER role, no IP access list so MCK operator pods can reach OM).
# Override them on the make command line, e.g.:
#   make test-mongodb \
#     MCK_TEST_PUBLIC_KEY=... \
#     MCK_TEST_PRIVATE_KEY=... \
#     MCK_TEST_ORG_ID=...
MCK_TEST_PUBLIC_KEY  ?= <your-public-key>
MCK_TEST_PRIVATE_KEY ?= <your-private-key>
MCK_TEST_ORG_ID      ?= <your-org-id>
MCK_TEST_PROJECT     ?= test-backup
MCK_TEST_RS_NAME     ?= test-rs
MCK_TEST_RS_VERSION  ?= 6.0.5

# ── Colors & Logging ─────────────────────────────────────────
RESET   := \033[0m
BOLD    := \033[1m
GREEN   := \033[32m
YELLOW  := \033[33m
CYAN    := \033[36m
RED     := \033[31m

define log-info
	@printf '$(CYAN)$(BOLD)[INFO]$(RESET)  %s\n' $(1)
endef
define log-ok
	@printf '$(GREEN)$(BOLD)[OK]$(RESET)    %s\n' $(1)
endef
define log-warn
	@printf '$(YELLOW)$(BOLD)[WARN]$(RESET)  %s\n' $(1)
endef
define log-error
	@printf '$(RED)$(BOLD)[ERROR]$(RESET) %s\n' $(1)
endef

# ── Phony Targets ────────────────────────────────────────────
.PHONY: all \
        preflight \
        cluster cluster-delete cluster-status \
        mck mck-namespace mck-operator \
        ops-manager \
        crossplane crossplane-install crossplane-wait \
        headlamp headlamp-namespace headlamp-deploy headlamp-token \
        minio minio-namespace minio-deploy minio-bucket \
        glauth glauth-namespace glauth-deploy \
        test-mongodb test-mongodb-whitelist \
        status teardown help

# ============================================================
#  DEFAULT TARGET
# ============================================================
all: preflight cluster mck ops-manager crossplane headlamp minio glauth
	$(call log-ok,'Full stack bootstrap complete!')
	@echo ""
	@$(MAKE) status

# ============================================================
#  PREFLIGHT CHECKS
# ============================================================
preflight:
	$(call log-info,'Running preflight checks...')
	@command -v docker  >/dev/null 2>&1 || { printf '$(RED)$(BOLD)[ERROR]$(RESET) %s\n' 'docker is not installed.'; exit 1; }
	@command -v kind    >/dev/null 2>&1 || { printf '$(RED)$(BOLD)[ERROR]$(RESET) %s\n' 'kind is not installed. Run: brew install kind'; exit 1; }
	@command -v kubectl >/dev/null 2>&1 || { printf '$(RED)$(BOLD)[ERROR]$(RESET) %s\n' 'kubectl is not installed. Run: brew install kubectl'; exit 1; }
	@command -v helm    >/dev/null 2>&1 || { printf '$(RED)$(BOLD)[ERROR]$(RESET) %s\n' 'helm is not installed. Run: brew install helm'; exit 1; }
	@docker info >/dev/null 2>&1       || { printf '$(RED)$(BOLD)[ERROR]$(RESET) %s\n' 'Docker daemon is not running.'; exit 1; }
	$(call log-ok,'All preflight checks passed.')

# ============================================================
#  KIND CLUSTER
# ============================================================
cluster: preflight
	$(call log-info,'Checking for existing kind cluster...')
	@if kind get clusters 2>/dev/null | grep -q '^$(CLUSTER_NAME)$$'; then \
		printf '$(YELLOW)$(BOLD)[WARN]$(RESET)  Cluster $(CLUSTER_NAME) already exists — skipping.\n'; \
	else \
		printf '$(CYAN)$(BOLD)[INFO]$(RESET)  Creating kind cluster $(CLUSTER_NAME)...\n'; \
		$(MAKE) _kind-config; \
		kind create cluster \
			--name $(CLUSTER_NAME) \
			--config $(KIND_CONFIG) \
			--wait 120s; \
		printf '$(GREEN)$(BOLD)[OK]$(RESET)    Cluster $(CLUSTER_NAME) is ready.\n'; \
	fi

# Generates a kind config inline (no external file required)
_kind-config:
	@printf '%s\n' \
		'kind: Cluster' \
		'apiVersion: kind.x-k8s.io/v1alpha4' \
		'name: $(CLUSTER_NAME)' \
		'nodes:' \
		'  - role: control-plane' \
		'    kubeadmConfigPatches:' \
		'      - |' \
		'        kind: InitConfiguration' \
		'        nodeRegistration:' \
		'          kubeletExtraArgs:' \
		'            node-labels: "ingress-ready=true"' \
		'    extraPortMappings:' \
		'      # Ingress HTTP / HTTPS' \
		'      - containerPort: 80' \
		'        hostPort: 8080' \
		'        protocol: TCP' \
		'      - containerPort: 443' \
		'        hostPort: 8443' \
		'        protocol: TCP' \
		'      # Headlamp UI' \
		'      - containerPort: 30080' \
		'        hostPort: 30080' \
		'        protocol: TCP' \
		'      # MongoDB Ops Manager (NodePort)' \
		'      - containerPort: 32001' \
		'        hostPort: 32001' \
		'        protocol: TCP' \
		'      # MinIO S3-compatible storage' \
		'      - containerPort: 30090' \
		'        hostPort: 30090' \
		'        protocol: TCP' \
		'      # MinIO Console' \
		'      - containerPort: 30091' \
		'        hostPort: 30091' \
		'        protocol: TCP' \
		'      # GLAuth LDAP server' \
		'      - containerPort: 30092' \
		'        hostPort: 30092' \
		'        protocol: TCP' \
		'      # Spare slots for upcoming applications' \
		'      - containerPort: 30093' \
		'        hostPort: 30093' \
		'        protocol: TCP' \
		'      - containerPort: 30094' \
		'        hostPort: 30094' \
		'        protocol: TCP' \
		'  - role: worker' \
		'  - role: worker' \
		> $(KIND_CONFIG)
	$(call log-ok,'kind-config.yaml written.')

cluster-delete:
	$(call log-warn,'Deleting kind cluster $(CLUSTER_NAME)...')
	@kind delete cluster --name $(CLUSTER_NAME)
	$(call log-ok,'Cluster deleted.')

cluster-status:
	@kind get clusters
	@kubectl cluster-info --context kind-$(CLUSTER_NAME)

# ============================================================
#  MONGODB CONTROLLERS FOR KUBERNETES v1.7.0
#  Image : quay.io/mongodb/mongodb-kubernetes:1.7.0
#  Chart : mongodb/mongodb-kubernetes
#  Repo  : https://github.com/mongodb/mongodb-kubernetes
# ============================================================
mck: mck-namespace mck-operator
	$(call log-ok,'MongoDB Controllers for Kubernetes Operator ready.')

mck-namespace:
	$(call log-info,'Ensuring namespace $(MCK_NAMESPACE) exists...')
	@kubectl get namespace $(MCK_NAMESPACE) >/dev/null 2>&1 \
		|| kubectl create namespace $(MCK_NAMESPACE)
	$(call log-ok,'Namespace $(MCK_NAMESPACE) ready.')

mck-operator: mck-namespace
	$(call log-info,'Adding MongoDB Helm repo...')
	@helm repo add mongodb $(MONGODB_HELM_REPO) --force-update >/dev/null
	@helm repo update >/dev/null
	$(call log-info,'Installing MongoDB Controllers for Kubernetes v$(MCK_VERSION)...')
	@helm upgrade --install \
		mongodb-kubernetes \
		mongodb/mongodb-kubernetes \
		--namespace $(MCK_NAMESPACE) \
		--version $(MCK_VERSION) \
		--set operator.watchNamespace="$(MCK_NAMESPACE)" \
		--wait \
		--timeout 5m
	$(call log-info,'Waiting for mongodb-kubernetes-operator pod...')
	@kubectl rollout status deployment/mongodb-kubernetes-operator \
		--namespace $(MCK_NAMESPACE) \
		--timeout=120s
	$(call log-ok,'MongoDB Controllers for Kubernetes v$(MCK_VERSION) installed and running.')

# ============================================================
#  MONGODB OPS MANAGER
#  Deploys the MongoDBOpsManager CR which provisions its own
#  application database replica set — no separate
#  MongoDBCommunity resource is needed.
# ============================================================
ops-manager: mck-operator
	$(call log-info,'Deploying MongoDBOpsManager CR v$(OPS_MANAGER_VERSION)...')
	@kubectl apply -n $(MCK_NAMESPACE) -f - <<< '{"apiVersion":"v1","kind":"Secret","metadata":{"name":"ops-manager-admin-secret","namespace":"$(MCK_NAMESPACE)"},"type":"Opaque","stringData":{"Username":"admin@example.com","Password":"OpsManager@SecurePass123!","FirstName":"Admin","LastName":"User"}}'
	@kubectl apply -n $(MCK_NAMESPACE) -f - <<< '{"apiVersion":"mongodb.com/v1","kind":"MongoDBOpsManager","metadata":{"name":"ops-manager","namespace":"$(MCK_NAMESPACE)"},"spec":{"version":"$(OPS_MANAGER_VERSION)","adminCredentials":"ops-manager-admin-secret","replicas":1,"externalConnectivity":{"type":"NodePort","port":32001},"configuration":{"mms.ignoreUserSuffix":"true"},"statefulSet":{"spec":{"template":{"spec":{"containers":[{"name":"mongodb-ops-manager","resources":{"requests":{"cpu":"0.25","memory":"800Mi"},"limits":{"cpu":"1.0","memory":"2Gi"}}}]}}}},"applicationDatabase":{"members":3,"topology":"SingleCluster","version":"$(OPS_MANAGER_APPDB)"},"backup":{"enabled":true,"statefulSet":{"spec":{"template":{"spec":{"containers":[{"name":"mongodb-backup-daemon","resources":{"requests":{"cpu":"0.25","memory":"800Mi"},"limits":{"cpu":"0.5","memory":"1Gi"}}}]}}}}}}}'
	$(call log-info,'Waiting for Ops Manager to reach Running phase (this may take ~8-12 min)...')
	@kubectl wait mongodbopsmanager/ops-manager \
		--namespace $(MCK_NAMESPACE) \
		--for=jsonpath='{.status.opsManager.phase}'=Running \
		--timeout=900s \
	  || { printf '$(YELLOW)$(BOLD)[WARN]$(RESET)  Ops Manager not yet Running — check: kubectl get mongodbopsmanager -n $(MCK_NAMESPACE)\n'; }
	$(call log-ok,'Ops Manager v$(OPS_MANAGER_VERSION) deployed. Access via NodePort 32001.')

# ============================================================
#  CROSSPLANE
# ============================================================
crossplane: crossplane-install crossplane-wait
	$(call log-ok,'Crossplane v$(CROSSPLANE_VERSION) is ready.')

crossplane-install:
	$(call log-info,'Ensuring namespace $(CROSSPLANE_NAMESPACE) exists...')
	@kubectl get namespace $(CROSSPLANE_NAMESPACE) >/dev/null 2>&1 \
		|| kubectl create namespace $(CROSSPLANE_NAMESPACE)
	$(call log-info,'Adding Crossplane Helm repo...')
	@helm repo add crossplane-stable https://charts.crossplane.io/stable --force-update >/dev/null
	@helm repo update >/dev/null
	$(call log-info,'Installing Crossplane v$(CROSSPLANE_VERSION)...')
	@helm upgrade --install crossplane \
		crossplane-stable/crossplane \
		--namespace $(CROSSPLANE_NAMESPACE) \
		--version $(CROSSPLANE_VERSION) \
		--set args='{--debug}' \
		--set resourcesCrossplane.limits.cpu=500m \
		--set resourcesCrossplane.limits.memory=256Mi \
		--set resourcesCrossplane.requests.cpu=100m \
		--set resourcesCrossplane.requests.memory=128Mi \
		--wait \
		--timeout 5m
	$(call log-ok,'Crossplane Helm release installed.')

crossplane-wait:
	$(call log-info,'Waiting for Crossplane pods...')
	@kubectl rollout status deployment/crossplane \
		--namespace $(CROSSPLANE_NAMESPACE) \
		--timeout=120s
	@kubectl rollout status deployment/crossplane-rbac-manager \
		--namespace $(CROSSPLANE_NAMESPACE) \
		--timeout=120s
	$(call log-ok,'Crossplane pods are healthy.')

# ============================================================
#  HEADLAMP
# ============================================================
headlamp: headlamp-namespace headlamp-deploy headlamp-token
	$(call log-ok,'Headlamp deployed. Access via NodePort 30080.')

headlamp-namespace:
	$(call log-info,'Ensuring namespace $(HEADLAMP_NAMESPACE) exists...')
	@kubectl get namespace $(HEADLAMP_NAMESPACE) >/dev/null 2>&1 \
		|| kubectl create namespace $(HEADLAMP_NAMESPACE)
	$(call log-ok,'Namespace $(HEADLAMP_NAMESPACE) ready.')

headlamp-deploy: headlamp-namespace
	$(call log-info,'Creating Headlamp ServiceAccount and RBAC and Deployment...')
	@printf '%s\n' \
		'apiVersion: v1' \
		'kind: ServiceAccount' \
		'metadata:' \
		'  name: headlamp-admin' \
		'  namespace: $(HEADLAMP_NAMESPACE)' \
		'---' \
		'apiVersion: v1' \
		'kind: Secret' \
		'metadata:' \
		'  name: headlamp-admin-token' \
		'  namespace: $(HEADLAMP_NAMESPACE)' \
		'  annotations:' \
		'    kubernetes.io/service-account.name: headlamp-admin' \
		'type: kubernetes.io/service-account-token' \
		'---' \
		'apiVersion: rbac.authorization.k8s.io/v1' \
		'kind: ClusterRoleBinding' \
		'metadata:' \
		'  name: headlamp-admin' \
		'roleRef:' \
		'  apiGroup: rbac.authorization.k8s.io' \
		'  kind: ClusterRole' \
		'  name: cluster-admin' \
		'subjects:' \
		'  - kind: ServiceAccount' \
		'    name: headlamp-admin' \
		'    namespace: $(HEADLAMP_NAMESPACE)' \
		'---' \
		'apiVersion: apps/v1' \
		'kind: Deployment' \
		'metadata:' \
		'  name: headlamp' \
		'  namespace: $(HEADLAMP_NAMESPACE)' \
		'  labels:' \
		'    app: headlamp' \
		'spec:' \
		'  replicas: 1' \
		'  selector:' \
		'    matchLabels:' \
		'      app: headlamp' \
		'  template:' \
		'    metadata:' \
		'      labels:' \
		'        app: headlamp' \
		'    spec:' \
		'      serviceAccountName: headlamp-admin' \
		'      containers:' \
		'        - name: headlamp' \
		'          image: ghcr.io/headlamp-k8s/headlamp:latest' \
		'          args:' \
		'            - "-in-cluster"' \
		'            - "-plugins-dir=/headlamp/plugins"' \
		'          ports:' \
		'            - containerPort: 4466' \
		'              name: http' \
		'          resources:' \
		'            requests:' \
		'              cpu: "100m"' \
		'              memory: "128Mi"' \
		'            limits:' \
		'              cpu: "500m"' \
		'              memory: "256Mi"' \
		'---' \
		'apiVersion: v1' \
		'kind: Service' \
		'metadata:' \
		'  name: headlamp' \
		'  namespace: $(HEADLAMP_NAMESPACE)' \
		'spec:' \
		'  type: NodePort' \
		'  selector:' \
		'    app: headlamp' \
		'  ports:' \
		'    - port: 80' \
		'      targetPort: 4466' \
		'      nodePort: 30080' \
		'      protocol: TCP' \
		| kubectl apply -f -
	$(call log-info,'Waiting for Headlamp deployment...')
	@kubectl rollout status deployment/headlamp \
		--namespace $(HEADLAMP_NAMESPACE) \
		--timeout=120s
	$(call log-ok,'Headlamp deployment is running.')

headlamp-token:
	$(call log-info,'Retrieving permanent Headlamp auth token...')
	@echo ""
	@printf '$(BOLD)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)\n'
	@printf '$(BOLD)   Headlamp Auth Token (permanent)$(RESET)\n'
	@printf '$(BOLD)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)\n'
	@echo ""
	@kubectl get secret headlamp-admin-token \
		--namespace $(HEADLAMP_NAMESPACE) \
		-o jsonpath='{.data.token}' | base64 -d
	@echo ""
	@echo ""

# ============================================================
#  MINIO — Lightweight S3-compatible object storage
#  Used as the S3 blockstore backend for Ops Manager backups.
#  API  : NodePort 30090  (S3 endpoint)
#  Console : NodePort 30091
# ============================================================
minio: minio-namespace minio-deploy minio-bucket
	$(call log-ok,'MinIO deployed. S3 API on port 30090 — Console on port 30091.')

minio-namespace:
	$(call log-info,'Ensuring namespace $(MINIO_NAMESPACE) exists...')
	@kubectl get namespace $(MINIO_NAMESPACE) >/dev/null 2>&1 \
		|| kubectl create namespace $(MINIO_NAMESPACE)
	$(call log-ok,'Namespace $(MINIO_NAMESPACE) ready.')

minio-deploy: minio-namespace
	$(call log-info,'Deploying MinIO (single-node) for S3-compatible storage...')
	@printf '%s\n' \
		'apiVersion: v1' \
		'kind: PersistentVolumeClaim' \
		'metadata:' \
		'  name: minio-data' \
		'  namespace: $(MINIO_NAMESPACE)' \
		'spec:' \
		'  accessModes:' \
		'    - ReadWriteOnce' \
		'  resources:' \
		'    requests:' \
		'      storage: 3Gi' \
		'---' \
		'apiVersion: apps/v1' \
		'kind: Deployment' \
		'metadata:' \
		'  name: minio' \
		'  namespace: $(MINIO_NAMESPACE)' \
		'  labels:' \
		'    app: minio' \
		'spec:' \
		'  replicas: 1' \
		'  selector:' \
		'    matchLabels:' \
		'      app: minio' \
		'  strategy:' \
		'    type: Recreate' \
		'  template:' \
		'    metadata:' \
		'      labels:' \
		'        app: minio' \
		'    spec:' \
		'      containers:' \
		'        - name: minio' \
		'          image: quay.io/minio/minio:latest' \
		'          args:' \
		'            - server' \
		'            - /data' \
		'            - --console-address' \
		'            - ":9001"' \
		'          env:' \
		'            - name: MINIO_ROOT_USER' \
		'              value: "$(MINIO_ROOT_USER)"' \
		'            - name: MINIO_ROOT_PASSWORD' \
		'              value: "$(MINIO_ROOT_PASSWORD)"' \
		'          ports:' \
		'            - containerPort: 9000' \
		'              name: s3' \
		'            - containerPort: 9001' \
		'              name: console' \
		'          resources:' \
		'            requests:' \
		'              cpu: "100m"' \
		'              memory: "256Mi"' \
		'            limits:' \
		'              cpu: "500m"' \
		'              memory: "1Gi"' \
		'          volumeMounts:' \
		'            - name: data' \
		'              mountPath: /data' \
		'          readinessProbe:' \
		'            httpGet:' \
		'              path: /minio/health/ready' \
		'              port: 9000' \
		'            initialDelaySeconds: 10' \
		'            periodSeconds: 10' \
		'          livenessProbe:' \
		'            httpGet:' \
		'              path: /minio/health/live' \
		'              port: 9000' \
		'            initialDelaySeconds: 10' \
		'            periodSeconds: 10' \
		'      volumes:' \
		'        - name: data' \
		'          persistentVolumeClaim:' \
		'            claimName: minio-data' \
		'---' \
		'apiVersion: v1' \
		'kind: Service' \
		'metadata:' \
		'  name: minio' \
		'  namespace: $(MINIO_NAMESPACE)' \
		'spec:' \
		'  type: NodePort' \
		'  selector:' \
		'    app: minio' \
		'  ports:' \
		'    - name: s3' \
		'      port: 9000' \
		'      targetPort: 9000' \
		'      nodePort: 30090' \
		'      protocol: TCP' \
		'    - name: console' \
		'      port: 9001' \
		'      targetPort: 9001' \
		'      nodePort: 30091' \
		'      protocol: TCP' \
		| kubectl apply -f -
	$(call log-info,'Waiting for MinIO deployment...')
	@kubectl rollout status deployment/minio \
		--namespace $(MINIO_NAMESPACE) \
		--timeout=120s
	$(call log-ok,'MinIO deployment is running.')

minio-bucket: minio-deploy
	$(call log-info,'Creating default bucket $(MINIO_BUCKET) via MinIO mc...')
	@printf '%s\n' \
		'apiVersion: batch/v1' \
		'kind: Job' \
		'metadata:' \
		'  name: minio-create-bucket' \
		'  namespace: $(MINIO_NAMESPACE)' \
		'spec:' \
		'  backoffLimit: 3' \
		'  ttlSecondsAfterFinished: 120' \
		'  template:' \
		'    spec:' \
		'      restartPolicy: OnFailure' \
		'      containers:' \
		'        - name: mc' \
		'          image: quay.io/minio/mc:latest' \
		'          command:' \
		'            - /bin/sh' \
		'            - -c' \
		'            - |' \
		'              mc alias set local http://minio.$(MINIO_NAMESPACE).svc.cluster.local:9000 $(MINIO_ROOT_USER) $(MINIO_ROOT_PASSWORD) && mc mb --ignore-existing local/$(MINIO_BUCKET)' \
		| kubectl apply -f -
	@printf '$(CYAN)$(BOLD)[INFO]$(RESET)  Waiting for bucket creation job to complete...\n'
	@kubectl wait job/minio-create-bucket \
		--namespace $(MINIO_NAMESPACE) \
		--for=condition=complete \
		--timeout=120s 2>/dev/null \
	  || { printf '$(YELLOW)$(BOLD)[WARN]$(RESET)  Bucket job not yet complete — check: kubectl logs -n $(MINIO_NAMESPACE) job/minio-create-bucket\n'; }
	$(call log-ok,'Bucket $(MINIO_BUCKET) is ready.')
	@echo ""
	@printf '$(BOLD)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)\n'
	@printf '$(BOLD)   MinIO S3 — Ops Manager Blockstore Config$(RESET)\n'
	@printf '$(BOLD)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)\n'
	@echo ""
	@printf '  S3 Endpoint    : http://minio.$(MINIO_NAMESPACE).svc.cluster.local:9000\n'
	@printf '  Bucket         : $(MINIO_BUCKET)\n'
	@printf '  Access Key     : $(MINIO_ROOT_USER)\n'
	@printf '  Secret Key     : $(MINIO_ROOT_PASSWORD)\n'
	@printf '  Region         : us-east-1 (default)\n'
	@printf '  Path Style     : true\n'
	@echo ""
	@printf '  External S3 API   : http://localhost:30090\n'
	@printf '  External Console  : http://localhost:30091\n'
	@echo ""

# ============================================================
#  GLAUTH — Lightweight LDAP server
#  Used to test Ops Manager LDAP integration and Crossplane
#  LdapGroupMapping managed resources.
#  LDAP : glauth.glauth.svc.cluster.local:3893
#  LDAPS: glauth.glauth.svc.cluster.local:3894 (not enabled)
# ============================================================
glauth: glauth-namespace glauth-deploy
	$(call log-ok,'GLAuth LDAP server deployed. LDAP on glauth.$(GLAUTH_NAMESPACE).svc.cluster.local:3893.')

glauth-namespace:
	$(call log-info,'Ensuring namespace $(GLAUTH_NAMESPACE) exists...')
	@kubectl get namespace $(GLAUTH_NAMESPACE) >/dev/null 2>&1 \
		|| kubectl create namespace $(GLAUTH_NAMESPACE)
	$(call log-ok,'Namespace $(GLAUTH_NAMESPACE) ready.')

glauth-deploy: glauth-namespace
	$(call log-info,'Deploying GLAuth lightweight LDAP server...')
	@printf '%s\n' \
		'apiVersion: v1' \
		'kind: ConfigMap' \
		'metadata:' \
		'  name: glauth-config' \
		'  namespace: $(GLAUTH_NAMESPACE)' \
		'data:' \
		'  config.cfg: |' \
		'    [ldap]' \
		'      enabled = true' \
		'      listen = "0.0.0.0:3893"' \
		'    [ldaps]' \
		'      enabled = false' \
		'    [backend]' \
		'      datastore = "config"' \
		'      baseDN = "dc=opsmanager,dc=local"' \
		'    # ----- Admin user (binds for Ops Manager) -----' \
		'    [[users]]' \
		'      name = "serviceaccount"' \
		'      givenname = "Service"' \
		'      sn = "Account"' \
		'      mail = "svc@opsmanager.local"' \
		'      uidnumber = 5000' \
		'      primarygroup = 5501' \
		'      passsha256 = "048f996f64b3c4450f0e23c0483fa86983617daab669955e67aa3d700928dd20"' \
		'      # password: "ldapadmin"' \
		'      [[users.capabilities]]' \
		'        action = "search"' \
		'        object = "*"' \
		'    # ----- Ops Manager local admin (cn must match admin@example.com username) -----' \
		'    [[users]]' \
		'      name = "admin@example.com"' \
		'      givenname = "Admin"' \
		'      sn = "User"' \
		'      mail = "admin@example.com"' \
		'      uidnumber = 5004' \
		'      primarygroup = 5501' \
		'      passsha256 = "27fc432b8add4223c81245cbb21a0cd4c1b0dcebe99a318d9df30d1db3c2a32b"' \
		'      # password: "OpsManager@SecurePass123!"' \
		'      [[users.capabilities]]' \
		'        action = "search"' \
		'        object = "*"' \
		'    # ----- Test users -----' \
		'    [[users]]' \
		'      name = "opsadmin"' \
		'      givenname = "Ops"' \
		'      sn = "Admin"' \
		'      mail = "opsadmin@opsmanager.local"' \
		'      uidnumber = 5001' \
		'      primarygroup = 5501' \
		'      otherGroups = [5502]' \
		'      passsha256 = "048f996f64b3c4450f0e23c0483fa86983617daab669955e67aa3d700928dd20"' \
		'    [[users]]' \
		'      name = "developer"' \
		'      givenname = "Dev"' \
		'      sn = "User"' \
		'      mail = "developer@opsmanager.local"' \
		'      uidnumber = 5002' \
		'      primarygroup = 5502' \
		'      passsha256 = "048f996f64b3c4450f0e23c0483fa86983617daab669955e67aa3d700928dd20"' \
		'    [[users]]' \
		'      name = "readonly"' \
		'      givenname = "Read"' \
		'      sn = "Only"' \
		'      mail = "readonly@opsmanager.local"' \
		'      uidnumber = 5003' \
		'      primarygroup = 5503' \
		'      passsha256 = "048f996f64b3c4450f0e23c0483fa86983617daab669955e67aa3d700928dd20"' \
		'    # ----- Groups -----' \
		'    [[groups]]' \
		'      name = "opsmanager-admins"' \
		'      gidnumber = 5501' \
		'    [[groups]]' \
		'      name = "opsmanager-developers"' \
		'      gidnumber = 5502' \
		'    [[groups]]' \
		'      name = "opsmanager-readonly"' \
		'      gidnumber = 5503' \
		'---' \
		'apiVersion: apps/v1' \
		'kind: Deployment' \
		'metadata:' \
		'  name: glauth' \
		'  namespace: $(GLAUTH_NAMESPACE)' \
		'  labels:' \
		'    app: glauth' \
		'spec:' \
		'  replicas: 1' \
		'  selector:' \
		'    matchLabels:' \
		'      app: glauth' \
		'  template:' \
		'    metadata:' \
		'      labels:' \
		'        app: glauth' \
		'    spec:' \
		'      containers:' \
		'        - name: glauth' \
		'          image: glauth/glauth:latest' \
		'          command:' \
		'            - /app/glauth' \
		'            - -c' \
		'            - /config/config.cfg' \
		'          ports:' \
		'            - containerPort: 3893' \
		'              name: ldap' \
		'          resources:' \
		'            requests:' \
		'              cpu: "50m"' \
		'              memory: "32Mi"' \
		'            limits:' \
		'              cpu: "100m"' \
		'              memory: "64Mi"' \
		'          volumeMounts:' \
		'            - name: config' \
		'              mountPath: /config' \
		'              readOnly: true' \
		'      volumes:' \
		'        - name: config' \
		'          configMap:' \
		'            name: glauth-config' \
		'---' \
		'apiVersion: v1' \
		'kind: Service' \
		'metadata:' \
		'  name: glauth' \
		'  namespace: $(GLAUTH_NAMESPACE)' \
		'spec:' \
		'  type: NodePort' \
		'  selector:' \
		'    app: glauth' \
		'  ports:' \
		'    - name: ldap' \
		'      port: 389' \
		'      targetPort: 3893' \
		'      nodePort: 30092' \
		'      protocol: TCP' \
		| kubectl apply -f -
	$(call log-info,'Waiting for GLAuth deployment...')
	@kubectl rollout status deployment/glauth \
		--namespace $(GLAUTH_NAMESPACE) \
		--timeout=120s
	$(call log-ok,'GLAuth LDAP server is running.')
	@echo ""
	@printf '$(BOLD)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)\n'
	@printf '$(BOLD)   GLAuth LDAP — Ops Manager Config$(RESET)\n'
	@printf '$(BOLD)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)\n'
	@echo ""
	@printf '  LDAP Server    : glauth.$(GLAUTH_NAMESPACE).svc.cluster.local\n'
	@printf '  LDAP Port      : 389 (external: localhost:30092)\n'
	@printf '  Bind DN        : cn=serviceaccount,dc=opsmanager,dc=local\n'
	@printf '  Bind Password  : ldapadmin\n'
	@printf '  User Base DN   : dc=opsmanager,dc=local\n'
	@printf '  Group Base DN  : dc=opsmanager,dc=local\n'
	@printf '  User Filter    : (objectClass=posixAccount)\n'
	@printf '  Group Filter   : (objectClass=posixGroup)\n'
	@echo ""
	@printf '  Users (all passwords: ldapadmin):\n'
	@printf '    opsadmin   — groups: opsmanager-admins, opsmanager-developers\n'
	@printf '    developer  — groups: opsmanager-developers\n'
	@printf '    readonly   — groups: opsmanager-readonly\n'
	@echo ""

# ============================================================
#  TEST MONGODB CLUSTER
#  Deploys a single-member replica set in the mongodb namespace
#  via MCK. MCK auto-creates the Ops Manager project on first
#  reconcile. Adopt the project afterwards with the Crossplane
#  Project CR (examples/project.yaml).
#
#  Prerequisites: ops-manager must be Running, MCK installed,
#  and MCK_TEST_PUBLIC_KEY / MCK_TEST_PRIVATE_KEY set.
# ============================================================
# Adds pod/node CIDRs to the Ops Manager global API key access list so that
# pods inside the kind cluster can authenticate with GLOBAL_OWNER keys.
# Safe to re-run — POST is idempotent for existing entries (Ops Manager 409s are ignored).
test-mongodb-whitelist:
	$(call log-info,'Adding cluster CIDRs to Ops Manager global access list...')
	@curl -s --digest -u "$(MCK_TEST_PUBLIC_KEY):$(MCK_TEST_PRIVATE_KEY)" \
	  -X POST "http://localhost:32001/api/public/v1.0/admin/whitelist" \
	  -H "Content-Type: application/json" \
	  -d '{"cidrBlock": "10.244.0.0/16", "description": "pod network"}' >/dev/null || true
	@curl -s --digest -u "$(MCK_TEST_PUBLIC_KEY):$(MCK_TEST_PRIVATE_KEY)" \
	  -X POST "http://localhost:32001/api/public/v1.0/admin/whitelist" \
	  -H "Content-Type: application/json" \
	  -d '{"cidrBlock": "172.18.0.0/16", "description": "kind node network"}' >/dev/null || true
	$(call log-ok,'Global access list updated.')

test-mongodb: test-mongodb-whitelist
	$(call log-info,'Creating MCK credentials secret for $(MCK_TEST_RS_NAME)...')
	@kubectl create secret generic $(MCK_TEST_RS_NAME)-credentials \
		--namespace $(MCK_NAMESPACE) \
		--from-literal=publicKey=$(MCK_TEST_PUBLIC_KEY) \
		--from-literal=privateKey=$(MCK_TEST_PRIVATE_KEY) \
		--dry-run=client -o yaml | kubectl apply -f -
	$(call log-info,'Creating Ops Manager ConfigMap for $(MCK_TEST_RS_NAME)...')
	@kubectl create configmap $(MCK_TEST_RS_NAME)-config \
		--namespace $(MCK_NAMESPACE) \
		--from-literal=baseUrl=http://ops-manager-svc.$(MCK_NAMESPACE).svc.cluster.local:8080 \
		--from-literal=projectName=$(MCK_TEST_PROJECT) \
		--from-literal=orgId=$(MCK_TEST_ORG_ID) \
		--dry-run=client -o yaml | kubectl apply -f -
	$(call log-info,'Applying MongoDB ReplicaSet CR $(MCK_TEST_RS_NAME)...')
	@printf '%s\n' \
		'apiVersion: mongodb.com/v1' \
		'kind: MongoDB' \
		'metadata:' \
		'  name: $(MCK_TEST_RS_NAME)' \
		'  namespace: $(MCK_NAMESPACE)' \
		'spec:' \
		'  members: 1' \
		'  type: ReplicaSet' \
		'  version: "$(MCK_TEST_RS_VERSION)"' \
		'  opsManager:' \
		'    configMapRef:' \
		'      name: $(MCK_TEST_RS_NAME)-config' \
		'  credentials: $(MCK_TEST_RS_NAME)-credentials' \
		'  persistent: false' \
		'  backup:' \
		'    mode: enabled' \
		'    assignmentLabels:' \
		'      - "$(MCK_TEST_RS_NAME)"' \
		'  statefulSet:' \
		'    spec:' \
		'      template:' \
		'        spec:' \
		'          containers:' \
		'            - name: mongod' \
		'              resources:' \
		'                requests:' \
		'                  cpu: "100m"' \
		'                  memory: "200Mi"' \
		'                limits:' \
		'                  cpu: "500m"' \
		'                  memory: "400Mi"' \
		| kubectl apply -f -
	$(call log-ok,'MongoDB CR $(MCK_TEST_RS_NAME) applied. Watch: kubectl get mongodb $(MCK_TEST_RS_NAME) -n $(MCK_NAMESPACE) -w')

# ============================================================
#  STATUS OVERVIEW
# ============================================================
status:
	@echo ""
	@printf '$(BOLD)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)\n'
	@printf '$(BOLD)   Stack Status — kind-$(CLUSTER_NAME)$(RESET)\n'
	@printf '$(BOLD)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(RESET)\n'
	@echo ""
	@printf '$(CYAN)▸ Nodes$(RESET)\n'
	@kubectl get nodes -o wide
	@echo ""
	@printf '$(CYAN)▸ MongoDB Controllers for Kubernetes (namespace: $(MCK_NAMESPACE))$(RESET)\n'
	@kubectl get pods -n $(MCK_NAMESPACE)
	@echo ""
	@printf '$(CYAN)▸ Ops Manager$(RESET)\n'
	@kubectl get mongodbopsmanager -n $(MCK_NAMESPACE) 2>/dev/null || echo '  (MongoDBOpsManager CR not yet applied)'
	@echo ""
	@printf '$(CYAN)▸ Crossplane (namespace: $(CROSSPLANE_NAMESPACE))$(RESET)\n'
	@kubectl get pods -n $(CROSSPLANE_NAMESPACE)
	@echo ""
	@printf '$(CYAN)▸ Headlamp (namespace: $(HEADLAMP_NAMESPACE))$(RESET)\n'
	@kubectl get pods -n $(HEADLAMP_NAMESPACE) 2>/dev/null || echo '  (Headlamp not yet deployed)'
	@echo ""
	@printf '$(CYAN)▸ MinIO S3 Storage (namespace: $(MINIO_NAMESPACE))$(RESET)\n'
	@kubectl get pods -n $(MINIO_NAMESPACE) 2>/dev/null || echo '  (MinIO not yet deployed)'
	@echo ""
	@printf '$(CYAN)▸ GLAuth LDAP (namespace: $(GLAUTH_NAMESPACE))$(RESET)\n'
	@kubectl get pods -n $(GLAUTH_NAMESPACE) 2>/dev/null || echo '  (GLAuth not yet deployed)'
	@echo ""
	@printf '$(CYAN)▸ Access URLs$(RESET)\n'
	@printf '  Headlamp        : http://localhost:30080\n'
	@printf '  Ops Manager     : http://localhost:32001\n'
	@printf '  MinIO S3 API    : http://localhost:30090\n'
	@printf '  MinIO Console   : http://localhost:30091\n'
	@printf '  LDAP (in-cluster): glauth.$(GLAUTH_NAMESPACE).svc.cluster.local:389\n'
	@printf '  LDAP (external)  : localhost:30092\n'
	@echo ""

# ============================================================
#  TEARDOWN
# ============================================================
teardown:
	$(call log-warn,'Tearing down the entire stack...')
	@$(MAKE) cluster-delete
	@rm -f $(KIND_CONFIG)
	$(call log-ok,'Teardown complete. All resources removed.')

# ============================================================
#  HELP
# ============================================================
help:
	@echo ""
	@printf '$(BOLD)Usage:$(RESET)  make [target]\n\n'
	@printf '$(BOLD)Targets:$(RESET)\n'
	@printf '  $(GREEN)all$(RESET)              Run full bootstrap (cluster + operator + ops-manager + crossplane + headlamp + minio)\n'
	@printf '  $(GREEN)preflight$(RESET)        Verify required tools are installed\n'
	@printf '  $(GREEN)cluster$(RESET)          Create the kind cluster\n'
	@printf '  $(GREEN)mck$(RESET)              Install MongoDB Controllers for Kubernetes Operator\n'
	@printf '  $(GREEN)mck-operator$(RESET)     Install only the operator (no CRs)\n'
	@printf '  $(GREEN)ops-manager$(RESET)      Deploy MongoDBOpsManager CR (includes its own AppDB)\n'
	@printf '  $(GREEN)crossplane$(RESET)       Install Crossplane\n'
	@printf '  $(GREEN)headlamp$(RESET)         Deploy Headlamp dashboard + permanent auth token\n'
	@printf '  $(GREEN)headlamp-token$(RESET)   Print the Headlamp auth token\n'
	@printf '  $(GREEN)minio$(RESET)            Deploy MinIO S3 storage + create blockstore bucket\n'
	@printf '  $(GREEN)minio-bucket$(RESET)     Create the Ops Manager blockstore bucket only\n'
	@printf '  $(GREEN)glauth$(RESET)           Deploy GLAuth lightweight LDAP server\n'
	@printf '  $(GREEN)status$(RESET)           Show current status of all components\n'
	@printf '  $(GREEN)teardown$(RESET)         Delete the cluster and clean up all files\n'
	@printf '  $(GREEN)cluster-delete$(RESET)   Delete only the kind cluster\n'
	@echo ""
	@printf '$(BOLD)Configuration overrides (via environment variables):$(RESET)\n'
	@printf '  CLUSTER_NAME=$(YELLOW)$(CLUSTER_NAME)$(RESET)   MCK_VERSION=$(YELLOW)$(MCK_VERSION)$(RESET)   CROSSPLANE_VERSION=$(YELLOW)$(CROSSPLANE_VERSION)$(RESET)\n'
	@echo ""
