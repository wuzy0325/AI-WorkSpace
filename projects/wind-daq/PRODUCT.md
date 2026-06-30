# Product

## Register

product

## Users

Wind-DAQ is used by wind tunnel and lab measurement engineers who keep the application open during device setup, acquisition, calibration, traversal, storage, and reporting workflows. Users need dense operational feedback they can trust under time pressure, often while monitoring hardware behavior and experiment state.

## Product Purpose

Wind-DAQ is the desktop DAQ application for wind tunnel measurement workflows, rebuilt on Go, Vue 3, and Wails. It coordinates device orchestration, acquisition visibility, calibration/traversal workflows, recording, storage, and diagnostic feedback while keeping business logic in backend usecases and hardware adapters.

## Brand Personality

Instrument-grade, modern, calm. The interface should feel like credible measurement software for engineering workstations: stable, readable, state-rich, and low-friction rather than promotional or decorative.

## Anti-references

Avoid marketing landing-page patterns, oversized hero typography, decorative gradients, glassy data panels, neon SCADA nostalgia, fake CRT effects, low-density dashboards, and card stacks that hide measurement context. The UI should not look like a generic SaaS admin template or a 1990s industrial control panel.

## Design Principles

1. Prioritize measurement trust: numbers, timestamps, state, source, and units must be readable before chrome gets attention.
2. Make state explicit: device, acquisition, logging, calibration, traversal, error, empty, and offline states should be visible without guessing from button labels.
3. Preserve workflow density: operators need compact panels, useful defaults, and fast filtering rather than sparse presentation layouts.
4. Keep visual energy restrained: motion and color communicate state only; decoration must not reduce readability.
5. Respect system boundaries: frontend displays state and interactions, while hardware access, algorithms, and business rules stay in Go backend layers.

## Accessibility & Inclusion

Maintain readable contrast in both light and dark themes, preserve keyboard-focus visibility, avoid color-only status communication, support reduced-motion preferences, and prevent long operational text from overflowing or hiding actions at the target desktop window size.
