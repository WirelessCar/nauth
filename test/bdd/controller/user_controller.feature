@controller @user
Feature: User controller reconciliation
  The user controller reconciles User resources and enforces account linkage,
  status conditions, and deletion behavior.

  Background:
    Given a user namespace "test-namespace" exists
    And a User named "test-resource" exists in that namespace
    And the operator version is "0.0-SNAPSHOT"

  Scenario: Create or update user successfully
    Given the User specification is valid and references an existing Account
    When the user reconcile loop runs
    Then the User status condition is "True" with reason "Reconciled"
    And the User status operator version is "0.0-SNAPSHOT"
    And no warning events are recorded

  Scenario: Fail to create user without a valid account
    Given the User references a missing Account
    When the user reconcile loop runs
    Then the User status condition is "False" with reason "Errored"
    And a warning event includes "no account found"
    And reconciliation returns the same error

  Scenario: Delete user successfully
    Given the User is ready for deletion
    When the User is deleted and reconciliation runs
    Then the User status condition is "True" with reason "Reconciled"
    And the User resource is removed from the cluster
    And reconciliation completes without error

  Scenario: Delete user fails
    Given the User deletion cannot complete due to an external dependency error
    When the User is deleted and reconciliation runs
    Then the User status condition is "False" with reason "Errored"
    And a warning event includes the deletion error
    And reconciliation returns an error
    And the User resource still exists

  Scenario: Update user when operator version changes
    Given the User specification is valid and references an existing Account
    When the operator version changes to "1.1-SNAPSHOT" and reconciliation runs again
    Then the User status condition is "True" with reason "Reconciled"
    And the User status operator version is "1.1-SNAPSHOT"
    And no warning events are recorded

  Scenario: Reconcile request for a missing user
    Given no User exists for the reconcile request
    When the user reconcile loop runs
    Then reconciliation completes without error
    And no warning events are recorded

  Scenario: No-op reconcile when nothing has changed
    Given the User status observed generation matches the current generation
    And the User status operator version is "0.0-SNAPSHOT"
    When the user reconcile loop runs
    Then reconciliation completes without error
    And no warning events are recorded

  Scenario: Finalizer is added on first reconcile
    Given the User has no finalizers
    When the user reconcile loop runs
    Then the User includes the "user.nauth.io/finalizer" finalizer
