---
title: Troubleshooting nauth
description: Common issues and solutions for nauth operator
---

This guide covers common issues you might encounter when using nauth and how to resolve them.

## General Debugging

### Check Operator Status

```bash
# Check if the operator is running
kubectl get pods -n nauth-system

# View operator logs
kubectl logs -n nauth-system deployment/nauth-controller-manager

# Check operator events
kubectl get events -n nauth-system --sort-by='.lastTimestamp'
```

### Verify CRDs

```bash
# List installed CRDs
kubectl get crd | grep nauth.io

# Check CRD details
kubectl describe crd accounts.nauth.io
kubectl describe crd users.nauth.io
```

## Account Issues

### Account Not Creating

**Symptoms**: Account resource exists but NATS account is not created.

**Diagnosis**:
```bash
kubectl describe account <account-name>
kubectl get account <account-name> -o yaml
```

**Common Causes**:
- NATS operator connection issues
- Invalid account limits
- Missing RBAC permissions
- Operator signing key problems

**Solutions**:
```bash
# Check operator configuration
kubectl get configmap -n nauth-system

# Verify NATS connection
kubectl logs -n nauth-system deployment/nauth-controller-manager | grep -i nats

# Check account limits are valid
kubectl apply --dry-run=server -f account.yaml
```

### Account Status Shows Errors

**Check the conditions field**:
```bash
kubectl get account <account-name> -o jsonpath='{.status.conditions[*].message}'
```

**Common errors**:
- `"signing key not found"`: Operator needs proper JWT signing configuration
- `"connection refused"`: NATS server is not accessible
- `"invalid limits"`: Account limits exceed server maximums

## User Issues

### User Not Creating

**Symptoms**: User resource exists but no credentials are generated.

**Diagnosis**:
```bash
kubectl describe user <user-name>
kubectl get secrets | grep user-<user-name>
```

**Common Causes**:
- Referenced account doesn't exist
- Invalid permission patterns
- Account doesn't have capacity for new users

**Solutions**:
```bash
# Verify account exists
kubectl get account <account-name>

# Check user limits don't exceed account limits
kubectl get account <account-name> -o yaml | grep -A 10 limits

# Validate permission patterns
kubectl apply --dry-run=server -f user.yaml
```

### Credentials Not Working

**Symptoms**: Application can't connect using generated credentials.

**Diagnosis**:
```bash
# Check if secret was created
kubectl get secret user-<user-name>-creds

# Verify secret contents
kubectl get secret user-<user-name>-creds -o yaml

# Test NATS connection
nats pub --creds=/path/to/user.creds test.subject "hello"
```

**Solutions**:
- Verify NATS server endpoint is correct
- Check if account is properly configured in NATS
- Ensure user permissions allow the intended operations

## Permission Issues

### Permission Denied Errors

**Symptoms**: User can connect but can't publish/subscribe to subjects.

**Diagnosis**:
```bash
# Review user permissions
kubectl get user <user-name> -o yaml | grep -A 20 permissions

# Check NATS server logs for permission denials
kubectl logs -n nats-system deployment/nats
```

**Common Patterns**:
```yaml
# Too restrictive
permissions:
  pub:
    allow: ["specific.subject"]  # Only allows exact match
    
# Better approach  
permissions:
  pub:
    allow: ["app.>"]  # Allows all subjects under app.
```

### Subject Pattern Debugging

Test subject patterns:
```bash
# This matches:
# ✓ app.users.create
# ✓ app.orders.update
# ✗ system.admin
pattern: "app.>"

# This matches:
# ✓ events.user.login
# ✓ events.user.logout  
# ✗ events.system.restart
pattern: "events.user.*"
```

## Resource Limit Issues

### Hitting Account Limits

**Symptoms**: New users fail to create or connections are rejected.

**Diagnosis**:
```bash
# Check current account usage
kubectl get account <account-name> -o yaml | grep -A 10 status

# Monitor account metrics (if available)
kubectl top pods -n nauth-system
```

**Solutions**:
```yaml
# Increase account limits
spec:
  accountLimits:
    conn: 200  # Increase from 100
    exports: 20  # Increase exports if needed
```

### JetStream Storage Issues

**Symptoms**: Stream creation fails or storage warnings.

**Check JetStream limits**:
```bash
kubectl get account <account-name> -o yaml | grep -A 10 jetStreamLimits
```

**Adjust limits**:
```yaml
spec:
  jetStreamLimits:
    memStorage: 209715200   # 200MB
    diskStorage: 2147483648 # 2GB
    streams: 100
```

## Network and Connectivity

### NATS Server Connection Issues

**Symptoms**: Operator can't connect to NATS server.

**Diagnosis**:
```bash
# Check operator logs for connection errors
kubectl logs -n nauth-system deployment/nauth-controller-manager | grep -i "connection\|error"

# Test network connectivity
kubectl exec -n nauth-system deployment/nauth-controller-manager -- nc -z nats.nats-system 4222
```

**Solutions**:
- Verify NATS server is running and accessible
- Check network policies and firewall rules
- Ensure correct NATS server endpoint configuration

### DNS Resolution Issues

**Check DNS resolution**:
```bash
kubectl exec -n nauth-system deployment/nauth-controller-manager -- nslookup nats.nats-system
```

## RBAC and Security

### Permission Denied for CRD Operations

**Symptoms**: Operator can't create or update accounts/users.

**Check RBAC**:
```bash
kubectl describe clusterrole nauth-manager-role
kubectl describe clusterrolebinding nauth-manager-rolebinding
```

**Verify service account**:
```bash
kubectl get serviceaccount -n nauth-system
kubectl describe serviceaccount nauth-controller-manager -n nauth-system
```

## Useful Commands

### Debug Information Collection

```bash
#!/bin/bash
# Collect debug information

echo "=== Operator Status ==="
kubectl get pods -n nauth-system

echo "=== CRDs ==="
kubectl get crd | grep nauth.io

echo "=== Accounts ==="
kubectl get accounts --all-namespaces

echo "=== Users ==="  
kubectl get users --all-namespaces

echo "=== Recent Events ==="
kubectl get events --all-namespaces --sort-by='.lastTimestamp' | tail -20

echo "=== Operator Logs ==="
kubectl logs -n nauth-system deployment/nauth-controller-manager --tail=50
```

### Resource Cleanup

```bash
# Clean up test resources
kubectl delete users --all
kubectl delete accounts --all

# Restart operator if needed
kubectl rollout restart deployment/nauth-controller-manager -n nauth-system
```

## Getting Help

If you're still experiencing issues:

1. **Check the GitHub Issues**: [nauth issues](https://github.com/wirelesscar/nauth/issues)
2. **Collect debug information** using the script above
3. **Review operator logs** for specific error messages
4. **Test with minimal configurations** first