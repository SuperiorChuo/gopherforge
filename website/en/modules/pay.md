# Payments

The **pay service** implements the full order lifecycle: orders, checkout, refunds, callbacks and statistics.

## ⚠️ Capability boundary (read first)

**This release ships a mock payment channel only**, for isolated testing and demos (explicit `PAY_DEMO_MODE`; disabled by default in production). **The WeChat/Alipay adapters are not implemented** — configuring real merchant credentials does not enable real payments. The mock endpoint lets anyone with the checkout link mark an order as paid, so it must stay disabled outside isolated environments.

## Capabilities (same under the mock channel)

- **Orders**: internal order creation with idempotency keys and a strict state machine (pending / paid / closed).
- **Checkout**: controlled anonymous checkout page; the mock channel completes simulated payment.
- **Callbacks**: signature verification, duplicate notifications handled idempotently (repeat notices answer 200 without double-crediting).
- **Refunds**: partial / full refunds, over-refund rejection, closed-order rejection; terminal states are locked.
- **Statistics**: order and amount rollups.
- **Auth**: callback token (`PAY_CALLBACK_TOKEN`) and internal token gating; bad tokens get 401.

## Roadmap

Real-channel adapters (WeChat/Alipay) are **not part of this release**; do not plan on "merchant credentials = real payments".
