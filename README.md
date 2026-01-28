# system-design-lab
A hands-on lab for mastering large-scale system design through real implementations, load testing, and iterative refinement.

#directory_structure_reference

system-design-lab/
├── README.md
├── principles/
│   ├── design-invariants.md
│   ├── scalability-checklist.md
│   └── failure-models.md
│
├── systems/
│   ├── top-k-user-aggregation/
│   │   ├── README.md
│   │   ├── design/
│   │   │   ├── excalidraw.png
│   │   │   ├── assumptions.md
│   │   │   ├── api-contracts.md
│   │   │   ├── data-models.md
│   │   │   └── design-doc.md
│   │   │
│   │   ├── implementation/
│   │   │   ├── docker-compose.yml
│   │   │   ├── services/
│   │   │   │   ├── ingestion-service/
│   │   │   │   ├── aggregation-worker/
│   │   │   │   ├── api-service/
│   │   │   │   └── scheduler/
│   │   │   └── storage/
│   │   │
│   │   ├── load-test/
│   │   │   ├── scenarios.md
│   │   │   ├── k6/
│   │   │   └── test-results/
│   │   │
│   │   ├── failure-tests/
│   │   │   ├── chaos-scenarios.md
│   │   │   └── injected-failures/
│   │   │
│   │   └── lessons-learned.md
│   │
│   ├── news-feed/
│   ├── rate-limiter/
│   ├── messaging-system/
│   └── search-indexing/
│
├── tooling/
│   ├── load-generators/
│   ├── data-generators/
│   └── observability/
│
├── templates/
│   ├── system-design-doc-template.md
│   ├── system-readme-template.md
│   └── load-test-template.md
│
└── docs/
    ├── interview-notes.md
    ├── common-tradeoffs.md
    └── design-review-checklist.md