@bdd @account @smoke
Feature: Account reconcile for missing resources
  The controller should ignore reconcile requests for Accounts that no longer exist.

  Scenario: Reconcile request for a missing account
    Given no Account exists for the reconcile request
    When the account reconcile loop runs
    Then reconciliation completes without error
    And no warning events are recorded
