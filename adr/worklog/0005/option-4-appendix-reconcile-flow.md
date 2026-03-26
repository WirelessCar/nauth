# Option 4 - Reconcile flow

`Account` remains the only resource that renders the final account JWT.
`AccountExport` and `AccountImport` reconcile their own readiness and status, and contribute validated inputs to the
owning account.

```mermaid
flowchart TD
    A0([Account changed])
    E0([AccountExport changed])
    I0([AccountImport changed])
    A0 --> AC1[Reconcile Account]

    subgraph Export_Reconcile[AccountExport reconcile]
        E0 --> E1[Load AccountExport]
        E1 --> E2[Load exporting Account from accountRef]
        E2 --> E3{Exporting Account exists?}
        E3 -- No --> E4[Set Ready=False<br/>record missing dependency]
        E4 --> E5[Enqueue exporting Account]
        E3 -- Yes --> E6[Validate access block<br/>and rule set]
        E6 --> E7{Valid?}
        E7 -- No --> E4
        E7 -- Yes --> E8[Publish Ready=True<br/>record resolved contract metadata]
        E8 --> E5
    end

    subgraph Import_Reconcile[AccountImport reconcile]
        I0 --> I1[Load AccountImport]
        I1 --> I2[Load importing Account from accountRef]
        I2 --> I3[Load referenced AccountExport from exportRef]
        I3 --> I4[Verify AccountExport.spec.accountRef<br/>matches exporterAccountRef]
        I4 --> I5[Verify importing Account is allowed<br/>by export access policy]
        I5 --> I6[Resolve ruleBindings by name<br/>from export rules]
        I6 --> I7[Validate ruleBindings.expected<br/>against referenced export rules]
        I7 --> I8[Validate importOptions and resolve subject<br/>localSubject > localSubjectPrefix > export subject]
        I8 --> I9[Check duplicates, collisions,<br/>and importer-side narrowing rules]
        I9 --> I10{Dependency and contract valid?}
        I10 -- No --> I11[Set Ready=False<br/>record validation or dependency error]
        I11 --> I12[Enqueue importing Account]
        I10 -- Yes --> I13[Compare resolved import state<br/>with current export state]
        I13 --> I14{Compatible upstream change detected?}
        I14 -- No --> I15[Set Ready=True<br/>clear Drifted condition]
        I14 -- Yes --> I16{updatePolicy}
        I16 -- automatic --> I17[Adopt new resolved state<br/>set Ready=True]
        I16 -- manual --> I18[Set Drifted=True and Ready=False<br/>do not adopt changed state]
        I15 --> I12
        I17 --> I12
        I18 --> I12
    end

    E5 --> AC1
    I12 --> AC1

    subgraph Account_Reconcile[Account reconcile]
        AC1 --> AC2[Validate base Account spec]
        AC2 --> AC3[List AccountExports with matching accountRef]
        AC3 --> AC4[List AccountImports with matching accountRef]
        AC4 --> AC5[Include only Ready AccountExports<br/>and Ready AccountImports]
        AC5 --> AC6[Sort deterministically<br/>namespace, name, rule name]
        AC6 --> AC7[Merge base account + export rules + resolved imports]
        AC7 --> AC8{Conflicts after merge?}
        AC8 -- Yes --> AC9[Set Account NotReady/Degraded<br/>record conflict details]
        AC8 -- No --> AC10[Render final Account JWT]
        AC10 --> AC11[Persist Secret and update status]
        AC11 --> AC12[Set Account Ready=True]
    end
```

**Notes on the flow**

* `AccountExport` validates the exporter-owned contract: exporting account existence, access policy, unique rule names,
  valid rule definitions, and any export-only options.
* `AccountImport` validates the importer-owned binding: importing account existence, referenced export existence,
  `exporterAccountRef` match, allow-list membership, referenced rule existence, `ruleBindings[].expected`,
  importer-local `importOptions`, and subject/collision rules.
* `updatePolicy` only applies after the contract is otherwise valid. It does not weaken validation.
* `manual` means compatible upstream export changes are detected and surfaced as drift instead of being adopted.
  Until the import is updated to accept that change, it does not contribute to the rendered account JWT.

