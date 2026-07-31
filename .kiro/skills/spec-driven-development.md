# Spec-Driven Development

## Workflow Overview

Every significant feature or project follows this phased approach:

1. **Discovery** — Conversational requirements gathering
2. **Specification** — Produce formal spec documents
3. **Implementation** — Task-by-task with verification
4. **Documentation** — Generate decision log and build report

## Phase 1: Discovery

- Start by understanding the user's intent at a high level.
- Ask multi-choice questions in rounds of 3-5, narrowing scope each round.
- Cover: use case, scope, key features, technical constraints, naming, tooling preferences.
- Summarize all confirmed decisions before proposing an implementation plan.
- Present a full implementation plan with task breakdown for user approval before coding.

## Phase 2: Specification

After discovery is complete and the plan is approved, create `.kiro/specs/{feature-name}/`:

### requirements.md
- Functional requirements (FR-N.N format) grouped by feature area.
- Non-functional requirements (NFR-N.N) covering: performance, compatibility, security, extensibility.
- Each requirement uses MUST/SHOULD/MAY language (RFC 2119 style).

### design.md
- System architecture diagram (ASCII or mermaid).
- Package/module structure.
- Core interfaces with design rationale for each decision.
- Data flow descriptions.
- Dependency management strategy.
- Known constraints and how they're resolved (e.g., import cycles).

### tasks.md
- Ordered implementation tasks with clear objectives.
- Each task lists: objective, implementation guidance, files to create/modify.
- Tasks are marked [DONE] as completed.
- Dependencies between tasks are reflected in ordering.

## Phase 3: Implementation

- Create a task list before writing any code.
- Implement tasks in order, one at a time.
- After each task: verify (build, test, vet) before moving to the next.
- Mark tasks complete with context notes about what was done.
- If a task reveals a design issue, update the spec before continuing.

## Phase 4: Documentation

At project completion, generate:

### DECISIONS.md
- Every decision made during discovery, with:
  - The question asked
  - Options presented
  - User's choice
  - Rationale
- Summary table at the end for quick reference.

### REPORT.md
- Timeline breakdown (tasks with approximate time).
- Output metrics (files, lines of code, packages, dependencies).
- What was built (organized by layer/component).
- Challenges encountered and how they were resolved.
- Verification results (test output, build status).

## When to Use This Workflow

- New packages or libraries being built from scratch.
- Significant features that span multiple files and packages.
- Projects where requirements need exploration before implementation.

## When NOT to Use This Workflow

- Quick bug fixes or small changes (just do them directly).
- User explicitly asks to skip planning and just implement.
- Simple questions that need a direct answer, not a project.
