# What is Threadify?

**Threadify is workflow threading infrastructure for distributed systems with services, AI agents, and human-in-the-loop steps.**

Thread workflows across microservices, partner APIs, AI agents, and human actions into unified, observable timelines. See what's happening, react to workflow states, and enforce business rules—without rewriting your coordination logic.

---

## The Problem

Modern workflows are **hybrid**: they mix automated services, AI agents making decisions, and humans performing approvals or manual tasks. But there's no infrastructure to observe or coordinate these complete workflows.

**What you face today:**

- **Services** scatter logs across systems—DataDog shows technical traces, but you can't see business context
- **AI agents** make autonomous decisions—LangSmith traces agents, but doesn't show how they fit into broader workflows
- **Humans** perform manual steps—approval tools exist, but they're disconnected from automated workflows
- **Partners** participate via APIs—when integrations fail, it's a "blame game" with no shared visibility

You can orchestrate what you control (Temporal), observe your services (DataDog), or trace your agents (LangSmith)—but **nobody threads the complete workflow across all participant types**.

---

## The Solution

Threadify provides **workflow threading infrastructure** that:

### 1. **Threads All Workflow Participants**

Create unified timelines across:
- **Services**: Microservices, APIs, background jobs
- **AI Agents**: LLM-powered decisions with reasoning context
- **Humans**: Approvals, manual reviews, overrides
- **Partners**: External APIs, third-party services

### 2. **Observability + Coordination**

**Observe:** See complete workflow execution across all participants
**Coordinate:** React to workflow state changes via event handlers
**Enforce:** Validate workflows against contracts in real-time

### 3. **Non-Invasive Integration**

Works alongside your existing architecture:
- Coexists with Kafka, REST APIs, whatever you have
- No rewrites required—just instrument with 3 lines of code
- Optional contracts for governance when you need them

---

## How It Works

### Instrument Your Workflows

Add Threadify to any service, agent, or human step:

```javascript
// Service: Payment processing
thread.step('Payment Processing')
thread.context({ gateway: 'stripe', amount: 4500 })
thread.stop('success')

// AI Agent: Document analysis
thread.step('AI Document Review')
thread.context({ model: 'claude-sonnet-4', confidence: 0.94 })
thread.context({ reasoning: 'Detected 3 risk factors' }, { isPrivate: true })
thread.stop('success', { approved: true })

// Human: Manual approval
thread.step('Compliance Review')
thread.context({ assignee: 'jane@company.com' })
// ... human reviews ...
thread.stop('success', { approver: 'jane', decision: 'approved' })
```

### See Unified Timelines

View complete workflow execution across all participants:

```
Thread: loan_application_#482

1. Service: Application Submitted (50ms)
2. Service: Credit Check Complete (1.2s)
3. AI Agent: Risk Assessment (confidence: 0.87) (800ms)
4. Human: Manual Review (assigned to Jane) (2.5h)
5. Service: Contract Generation (200ms)
6. Partner: Bank Account Opening (Plaid) (3.1s)
7. Human: Customer Signature (DocuSign) (1.2d)
8. Service: Funds Disbursed (150ms)

Status: ✅ Complete in 1.3 days
Contract: ✅ All steps validated
SLA: ✅ Within 2-day target
```

### Coordinate Through Workflow State

React to workflow events with handlers:

```javascript
// Agent completes with low confidence → escalate to human
threadify.on_step_complete('ai_risk_assessment', async (thread) => {
  if (thread.context.confidence < 0.9) {
    await createHumanReviewTask(thread);
  }
});

// Human approves → trigger next service
threadify.on_step_complete('compliance_approval', async (thread) => {
  if (thread.context.approved) {
    await processLoanDisbursement(thread);
  }
});

// Service fails → notify partner
threadify.on_step_failed('payment_processing', async (thread) => {
  await notifyPartner({ 
    threadId: thread.id, 
    error: thread.context.error 
  });
});
```

### Enforce Business Rules

Define workflow contracts to validate execution:

```yaml
workflow: loan_approval
participants:
  - type: service
    name: loan_service
  - type: agent
    name: risk_analyzer
  - type: human
    role: underwriter

steps:
  - name: credit_check
    participant: loan_service
    required: true
    
  - name: ai_risk_assessment
    participant: risk_analyzer
    required: true
    
  - name: human_review
    participant: underwriter
    required_if: ai_risk_assessment.confidence < 0.95
    max_duration: 4h

sla:
  total_duration: 48h
  
validation:
  - rule: "High-risk loans require human approval"
    condition: "ai_risk_assessment.risk_score > 0.8 AND human_review.completed"
```

---

## Key Features

### Threading Infrastructure
- **Multi-participant workflows**: Services, agents, humans, partners
- **Business context**: Not just correlation IDs—thread meaningful business data
- **Privacy controls**: Fine-grained visibility (`isPrivate` flag per context)

### Observability
- **Unified timelines**: Complete workflow view across all participants
- **Real-time visibility**: See workflows as they execute
- **Search by context**: Find threads by customer ID, order ID, or any business data

### Coordination
- **Event handlers**: React to `step_complete`, `step_failed`, `timeout`
- **Workflow-aware**: Coordinate through business states, not raw events
- **Decentralized**: No central orchestrator—services remain autonomous

### Governance
- **Contracts**: Define expected workflow structure (optional)
- **Runtime validation**: Detect violations as they happen
- **SLA enforcement**: Automatic escalation on timeouts
- **Audit trails**: Immutable record of who did what, when

### Cross-Organization
- **Partner threading**: Invite partners to participate in threads
- **Shared visibility**: Both sides see complete workflow
- **Privacy first**: Each party controls what they expose

---

## Use Cases

### 1. Partner Integration Debugging

**Problem:** When payment processing fails, you spend hours correlating logs between your service and Stripe.

**Solution:** Thread the complete payment workflow across your service and Stripe. See exactly where it failed and why.

```
Payment Thread:
├─ Your Service: Payment Initiated (✓)
├─ Stripe: Payment Received (✓)
├─ Stripe: Fraud Check (✓ 1.8s)
└─ Stripe: Payment Failed (❌)
   └─ Reason: Fraud check exceeded 3s SLA

Root Cause: Stripe's fraud check timeout
Attribution: Clear evidence for SLA dispute
```

### 2. AI Agent Governance

**Problem:** AI agents make autonomous decisions in production, but you can't see their reasoning or ensure they follow business rules.

**Solution:** Thread agent decisions with confidence scores, reasoning, and human oversight requirements.

```
Underwriting Thread:
├─ AI Agent: Document Analysis (confidence: 0.65)
│  └─ Reasoning: "Missing 2 required documents"
├─ Human: Manual Review Required (flagged by agent)
│  └─ Reviewer: "Approved with conditions"
└─ Contract: ✅ Agent correctly escalated (confidence < 0.95)
```

### 3. Customer Support Visibility

**Problem:** Support agents waste time asking engineering "Where is order #12345?"

**Solution:** Thread customer workflows. Support sees complete timeline without bothering engineering.

```
Order #12345 Timeline:
├─ Service: Order Created (10:23 AM)
├─ Service: Payment Processed (10:24 AM)
├─ Partner: Inventory Reserved (Shopify, 10:25 AM)
├─ Human: Order Picked (Warehouse, 11:42 AM)
├─ Partner: Label Generated (FedEx, 11:45 AM)
└─ Status: In Transit (Expected: Dec 27)
```

### 4. Compliance & Audit

**Problem:** Regulators ask "Prove that high-risk decisions required human approval."

**Solution:** Contracts enforce rules. Audit trails prove compliance.

```
Thread: trade_execution_#891
Contract Validation:
✅ AI proposed trade (risk_score: 0.82)
✅ Human compliance approval required (risk > 0.8)
✅ Approval received within 30min SLA
✅ Trade executed with dual authorization

Audit Trail: Complete, immutable record available
```

### 5. Hybrid Workflows

**Problem:** Your workflow mixes automated services, AI agents, and human approvals—no tool observes the complete flow.

**Solution:** Threadify threads all three participant types into unified timelines.

```
Content Moderation Workflow:
├─ Service: User uploads content
├─ AI Agent: Flags potential violation (confidence: 0.78)
├─ Human: Moderator reviews
├─ Human: Overrides AI decision (approved)
├─ AI Agent: Learns from override
└─ Service: Content published
```

---

## What Makes Threadify Different?

### vs. Workflow Orchestration (Temporal, Step Functions)

| | Temporal | Threadify |
|---|---|---|
| **Control model** | Central orchestrator | Decentralized threading |
| **What it handles** | Workflows you define | Workflows you don't control |
| **Partner APIs** | ❌ Can't orchestrate | ✅ Can thread |
| **Human steps** | Limited | ✅ First-class participant |
| **AI agents** | ❌ Not designed for | ✅ Built-in support |
| **Adoption** | Requires rewrite | Add 3 lines of code |

**Use Temporal when:** Building new workflows, need central control
**Use Threadify when:** Threading existing workflows, need cross-org visibility

---

### vs. Observability Tools (DataDog, New Relic)

| | DataDog | Threadify |
|---|---|---|
| **What it observes** | Services only | Services + agents + humans |
| **Context** | Technical metrics | Business workflow context |
| **Coordination** | ❌ Alerts only | ✅ Reactive handlers |
| **Partner visibility** | ❌ Can't instrument | ✅ Partners thread too |
| **Contracts** | ❌ None | ✅ Runtime validation |

**Use DataDog when:** Infrastructure/performance debugging
**Use Threadify when:** Business workflow visibility + coordination

---

### vs. AI Agent Tools (LangSmith, LangGraph)

| | LangSmith | Threadify |
|---|---|---|
| **Agent tracing** | ✅ Detailed | ✅ With workflow context |
| **Service integration** | ❌ Limited | ✅ First-class |
| **Human-in-loop** | Basic | ✅ First-class participant |
| **Workflow coordination** | ❌ Not designed for | ✅ Core feature |
| **Cross-org** | ❌ Single org | ✅ Multi-org threading |

**Use LangSmith when:** Deep agent debugging only
**Use Threadify when:** Agents are part of broader workflows

---

## Getting Started

### 1. Install SDK

```bash
npm install @threadify/sdk
# or
pip install threadify-sdk
```

### 2. Create Your First Thread

```javascript
import { Thread } from '@threadify/sdk';

// Connect
Thread.connect({ key: 'YOUR_API_KEY' });

// Create thread
const thread = Thread.create();

// Instrument workflow
thread.step('Order Processing');
thread.context({ orderId: '12345', amount: 99.99 });
thread.stop('success');
```

### 3. View in Dashboard

Visit `threadify.dev/threads` to see your workflow timeline.

### 4. Add Coordination (Optional)

```javascript
import { threadify } from '@threadify/sdk';

// React to workflow states
threadify.on_step_complete('payment_processing', async (thread) => {
  await sendConfirmationEmail(thread.context);
});
```

### 5. Define Contract (Optional)

```yaml
# contract.yaml
workflow: order_fulfillment
steps:
  - name: payment_processing
    required: true
  - name: inventory_check
    required: true
  - name: shipping_label
    required: true
```

---

## Pricing

**Free Tier**
- 1,000 threads/month
- Basic dashboard
- Community support

**Developer** - $99/month
- 10,000 threads/month
- Event handlers
- Email support

**Team** - $499/month
- 100,000 threads/month
- Contracts & validation
- Priority support
- Team collaboration

**Enterprise** - Custom
- Unlimited threads
- Partner threading
- Dedicated support
- SLA guarantees

---

## Learn More

- **Documentation**: [docs.threadify.dev](https://docs.threadify.dev)
- **GitHub**: [github.com/threadify](https://github.com/threadify)
- **Community**: [discord.gg/threadify](https://discord.gg/threadify)
- **Examples**: [threadify.dev/examples](https://threadify.dev/examples)

---

## The Vision

**Every modern system is hybrid—code, AI agents, and humans working together.**

Threadify is workflow infrastructure for this new reality. Like how Stripe made payments infrastructure and Segment made customer data infrastructure, we're making workflow infrastructure for companies building with AI.

Thread any workflow. See what's happening. Coordinate next steps. Enforce business rules.

**Infrastructure for the AI era.**