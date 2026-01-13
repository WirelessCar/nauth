@controller @account
Feature: Account controller reconciliation
  The account controller reconciles Account resources and keeps status, events,
  and deletion behavior consistent with operator expectations.

  Background:
    Given the operator namespace "nauth-account-system" exists
    And an account namespace "test-namespace" exists
    And an Account named "test-resource" exists in that namespace
    And the operator version is "0.0-SNAPSHOT"

  Scenario: Create account successfully
    Given the Account specification is valid
    When the account reconcile loop runs
    Then the Account status condition is "True" with reason "Reconciled"
    And the Account status operator version is "0.0-SNAPSHOT"
    And no warning events are recorded

  Scenario: Create account fails
    Given the Account specification is invalid
    When the account reconcile loop runs
    Then the Account status condition is "False" with reason "Errored"
    And a warning event includes "failed to create the account: a test error"
    And reconciliation returns an error

  Scenario: Observe mode does not delete managed resources
    Given the Account is labeled with management policy "observe"
    When the Account is deleted and reconciliation runs
    Then the Account resource is removed from the cluster
    And no managed resources are deleted by the controller

  Scenario: Delete account successfully
    Given the Account is ready for deletion
    When the Account is deleted and reconciliation runs
    Then the Account resource is removed from the cluster
    And reconciliation completes without error

  Scenario: Delete account fails
    Given the Account deletion cannot complete due to an external dependency error
    When the Account is deleted and reconciliation runs
    Then the Account status condition is "False" with reason "Errored"
    And a warning event includes the deletion error
    And reconciliation returns an error
    And the Account resource still exists

  Scenario: Delete account is blocked when users still exist
    Given the Account has associated Users in the same namespace
    When the Account is deleted and reconciliation runs
    Then the Account status condition is "False" with reason "Errored"
    And a warning event includes "cannot delete an account with associated users"
    And reconciliation returns an error
    And the Account resource still exists

  Scenario: Observe mode reconciliation succeeds
    Given the Account is labeled with management policy "observe"
    When the account reconcile loop runs
    Then reconciliation completes without error

  Scenario: Update account when operator version changes
    Given the Account specification is valid
    When the operator version changes to "1.1-SNAPSHOT" and reconciliation runs
    Then the Account status condition is "True" with reason "Reconciled"
    And the Account status operator version is "1.1-SNAPSHOT"
    And no warning events are recorded

  Scenario: Reconcile request for a missing account
    Given no Account exists for the reconcile request
    When the account reconcile loop runs
    Then reconciliation completes without error
    And no warning events are recorded

  Scenario: No-op reconcile when nothing has changed
    Given the Account specification is valid
    And the Account status observed generation matches the current generation
    And the Account status operator version is "0.0-SNAPSHOT"
    When the account reconcile loop runs
    Then reconciliation completes without error
    And no warning events are recorded

  Scenario: Finalizer is added on first reconcile
    Given the Account specification is valid
    And the Account has no finalizers
    When the account reconcile loop runs
    Then the Account includes the "account.nauth.io/finalizer" finalizer
