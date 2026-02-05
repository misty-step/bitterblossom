---
name: naming-conventions
user-invocable: false
description: "Apply universal naming principles: avoid Manager/Helper/Util, use intention-revealing names, domain language, verb+noun functions."
allowed-tools:
  - Read
  - Grep
---

# Naming Conventions

Universal principles for clear, intention-revealing names. Language-agnostic.

## Core Principle

**Names reveal intent and domain, not implementation.**

- `users` not `userArray`
- `calculateTax` not `doTaxCalculation`
- `PaymentProcessor` not `PaymentManager`

## Prohibited Patterns

**NEVER:** Manager, Helper, Util, Misc, Common, temporal names (step1, doFirst)
**RARELY:** Service, Handler, Processor (acceptable when domain-specific)

## Positive Patterns

| Type | Pattern | Examples |
|------|---------|----------|
| Variables | Descriptive nouns | `activeUsers`, `totalRevenue` |
| Functions | Verb + noun | `calculateTotal`, `fetchUser`, `formatDate` |
| Booleans | Question prefix | `isActive`, `hasPermission`, `canEdit` |
| Classes | Singular noun | `User`, `PaymentProcessor`, `OrderRepository` |
| Collections | Plural nouns | `users`, `items`, `userById` (keyed) |

### Function Verbs

- Pure: describe transformation (`format`, `parse`, `convert`)
- Side effects: verb implies action (`save`, `send`, `fetch`, `update`, `delete`)
- Boolean returns: question prefix (`is`, `has`, `can`, `should`)
- Avoid: `get`, `do`, `handle`, `manage`, `process` (without context)

### Boolean Prefixes

- **is**: State (`isActive`, `isLoading`, `isValid`)
- **has**: Possession (`hasPermission`, `hasChildren`)
- **can**: Capability (`canEdit`, `canDelete`)
- **should**: Conditional (`shouldRefetch`, `shouldValidate`)

### Collections

- Plural indicates multiple: `users`, `selectedItems`
- Keyed: `userById`, `configByEnv`, `productsByCategory`
- Never: `userList`, `userArray`, `userCollection`

## Context-Aware Exceptions

- Framework patterns: `EventBus`, `RequestHandler` (convention)
- Repository pattern: `UserRepository` (suffix adds meaning)
- Factory pattern: `UserFactory` (pattern clarifies purpose)

## Philosophy

**"If you can't name it, you don't understand it."**

Naming forces design decisions. Bad names indicate unclear thinking. Use domain language. Names are for humans, not compilers.
