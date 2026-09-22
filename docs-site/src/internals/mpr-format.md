# MPR File Format

Mendix projects are stored in `.mpr` files, which are SQLite databases containing BSON-encoded model elements. This page provides an overview of the format; see [v1 vs v2](./mpr-v1-v2.md) for version-specific details.

## Structure

An MPR file is a standard SQLite database. Both format versions carry the same
two tables; v2 adds a third. Contents live in the `Unit` table in v1 and in
`mprcontents/` in v2 — **there is no separate contents table in either format.**

| Table | v1 | v2 | Holds |
|-------|----|----|-------|
| `Unit` | yes | yes | One row per document |
| `_MetaData` | yes | yes | Mendix product/build version and schema hash |
| `_Transaction` | no | yes | `LastTransactionID`, bumped on every unit write |

### Unit Table

The `Unit` table has one row per document. Its columns are identical in both
formats except for `Contents`, which only v1 has:

| Column | v1 | v2 | Description |
|--------|----|----|-------------|
| `UnitID` | yes | yes | Binary UUID identifying the document (.NET GUID byte order — see below) |
| `ContainerID` | yes | yes | Parent unit's `UnitID`; the project root is its own container |
| `ContainmentName` | yes | yes | Relationship name (e.g. `ProjectDocuments`), empty on the root |
| `TreeConflict` | yes | yes | Version-control conflict marker |
| `ContentsHash` | yes | yes | Base64 SHA-256 of the document BSON |
| `ContentsConflicts` | yes | yes | Version-control conflict marker for contents |
| `Contents` | **yes** | **no** | BSON blob containing the full document |

The row carries **no unit-type and no name column** — the seven above are all
there is. A document's type and name are read out of its BSON `$Type` and
`Name` fields, which is why listing units by type requires decoding contents
(`getTypeFromContents` in `modelsdk/mpr/reader_units.go`).

### Where Contents Live

In **v1**, document BSON is the `Unit.Contents` blob:

```sql
SELECT Contents FROM Unit WHERE UnitID = ?;          -- read
UPDATE Unit SET Contents = ? WHERE UnitID = ?;       -- write
```

In **v2**, `Unit.Contents` does not exist. Each document is a file under
`mprcontents/`, sharded two levels deep by the first four hex characters of its
UUID:

```
mprcontents/<XX>/<YY>/<UUID>.mxunit
```

The UUID in the path is the `UnitID` blob rendered in **.NET GUID byte order** —
the first three fields are little-endian, so blob `FADF10BF FF61 8842 8A63D53AE4522615`
becomes `bf10dffa-61ff-4288-8a63-d53ae4522615`. Writing a v2 unit updates
`Unit.ContentsHash` (and `_Transaction.LastTransactionID`) in SQLite after the
file lands.

## Unit Types

Every document has a type, carried in its BSON `$Type` field. It is not a
column on `Unit`, so identifying a document means decoding its contents:

| `$Type` | Document Type |
|---------|---------------|
| `DomainModels$DomainModel` | Domain model (entities, associations) |
| `DomainModels$ViewEntitySourceDocument` | OQL query for VIEW entities |
| `Microflows$Microflow` | Microflow definition |
| `Microflows$Nanoflow` | Nanoflow definition |
| `Pages$Page` | Page definition |
| `Pages$Layout` | Layout definition |
| `Pages$Snippet` | Snippet definition |
| `Pages$BuildingBlock` | Building block definition |
| `Enumerations$Enumeration` | Enumeration definition |
| `JavaActions$JavaAction` | Java action definition |
| `Security$ProjectSecurity` | Project security settings |
| `Security$ModuleSecurity` | Module security settings |
| `Navigation$NavigationDocument` | Navigation profile |
| `Settings$ProjectSettings` | Project settings |
| `BusinessEvents$BusinessEventService` | Business event service |

## BSON Document Structure

Every BSON document contains at minimum:

| Field | Type | Description |
|-------|------|-------------|
| `$ID` | Binary (UUID) | Unique identifier for this element |
| `$Type` | String | Fully qualified type name (using **storageName**, not qualifiedName) |

### ID Format

IDs are stored as BSON Binary subtype 0 (generic) containing UUID bytes:

```json
{
  "$ID": {
    "Subtype": 0,
    "Data": "base64-encoded-uuid"
  }
}
```

### Array Convention

Arrays in Mendix BSON have an integer count as the first element:

```json
{
  "Attributes": [
    3,              // Count/type prefix
    { ... },        // First attribute
    { ... }         // Second attribute
  ]
}
```

The prefix value (typically `2` or `3`) indicates the array type. When writing arrays, you must include this prefix. When parsing, skip the first element.

### Reference Types

| Reference Type | Storage Format | Example |
|---------------|----------------|---------|
| `BY_ID_REFERENCE` | Binary UUID | Index `AttributePointer` |
| `BY_NAME_REFERENCE` | Qualified name string | ValidationRule `Attribute` |
| `PART` | Embedded BSON object | Child objects serialized inline |

Using the wrong reference format causes Studio Pro to fail loading the model. Always check the metamodel reflection data to determine which format each property uses.

## Storage Names vs Qualified Names

The `$Type` field in BSON must use the **storageName**, not the **qualifiedName**. These are often identical but not always:

| qualifiedName (SDK) | storageName (BSON $Type) |
|---------------------|--------------------------|
| `DomainModels$Entity` | `DomainModels$EntityImpl` |
| `DomainModels$Index` | `DomainModels$EntityIndex` |

Using the wrong name causes `TypeCacheUnknownTypeException` when opening in Studio Pro. See [Storage Names](./storage-names.md) for more details.
