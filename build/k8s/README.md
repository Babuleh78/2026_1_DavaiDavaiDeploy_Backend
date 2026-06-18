# DDDance — Kubernetes Dev Manifests

Dev-окружение для namespace `dddance-dev`. Не использовать в production без настройки Secrets и PersistentVolumes.

## Требования

- kubectl 1.28+
- minikube или kind (или любой k8s кластер)
- metrics-server (для HPA — см. ниже)

## Применить все манифесты

```bash
# Создать namespace
kubectl apply -f build/k8s/namespace.yaml

# Применить всё
kubectl apply -f build/k8s/
```

Рекурсивное применение (включая поддиректории):

```bash
kubectl apply -R -f build/k8s/
```

## Проверить состояние

```bash
# Поды
kubectl get pods -n dddance-dev

# Сервисы
kubectl get svc -n dddance-dev

# HPA
kubectl get hpa -n dddance-dev

# Логи backend
kubectl logs -n dddance-dev deployment/backend -f

# Логи ML (api контейнер)
kubectl logs -n dddance-dev deployment/ml-service -c api -f

# Логи Celery worker (sidecar)
kubectl logs -n dddance-dev deployment/ml-service -c celery-worker -f
```

## Создать Secrets перед запуском

Secrets не включены в манифесты (не хранить в VCS). Создать вручную:

```bash
# Secrets для Go backend
kubectl create secret generic backend-secrets \
  --namespace=dddance-dev \
  --from-literal=db-password=YOUR_DB_PASSWORD \
  --from-literal=jwt-secret=YOUR_JWT_SECRET \
  --from-literal=ml-internal-token=YOUR_ML_TOKEN \
  --from-literal=redis-addr=redis://redis:6379

# Secrets для ML сервиса
kubectl create secret generic ml-secrets \
  --namespace=dddance-dev \
  --from-literal=s3-access-key=YOUR_S3_KEY \
  --from-literal=s3-secret-key=YOUR_S3_SECRET \
  --from-literal=ml-internal-token=YOUR_ML_TOKEN
```

## HPA (Horizontal Pod Autoscaler)

HPA требует установленного **metrics-server**:

```bash
# minikube
minikube addons enable metrics-server

# kind / bare k8s
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

TODO: без metrics-server HPA будет создан, но не будет масштабировать (статус `<unknown>`).

Целевые значения:
- backend: min 1 / max 3, CPU 70%
- ml-service: min 1 / max 2, CPU 80%

## Обновить образ

```bash
# Backend
kubectl set image deployment/backend backend=dddance-backend:v2 -n dddance-dev

# ML
kubectl set image deployment/ml-service api=dddance-ml:v2 celery-worker=dddance-ml:v2 -n dddance-dev
```

## Что НЕ настроено

| Компонент | Статус | Примечание |
|---|---|---|
| PersistentVolume для PostgreSQL | Не настроен | PostgreSQL не включён в манифесты — используйте внешний DB или добавьте StatefulSet |
| PersistentVolume для Redis | Не настроен | Аналогично |
| Production Secrets | Не настроен | Использовать Vault, Sealed Secrets или External Secrets Operator |
| TLS / HTTPS | Не настроен | Добавить cert-manager + TLS в Ingress |
| Kafka | Не настроен | Использовать Strimzi Operator или внешний Kafka |
| Ingress Controller | Нужен | Установить nginx-ingress: `minikube addons enable ingress` |
| gRPC TLS (auth/notifications) | Не настроен | Добавить cert-manager + ConfigMap с CA |
