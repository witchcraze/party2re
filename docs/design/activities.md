# Delayed Activities & Training Design

## Purpose

This document records the initial Go training slice. Its one-hour duration and 10 EXP reward are implementation choices, not verified legacy Party2 requirements. The legacy equivalent and inclusion in the reconstruction baseline require review; do not use this slice to infer delayed adventure or combat rules.

## Overview

Delayed activities represent time-based tasks that characters can undertake. When started, an activity takes a fixed real-time duration before its completion and rewards become available.

## Training Specification

### Domain Model
- `Type`: `"training"`
- `Duration`: 1 Hour (`time.Hour`)
- `ExperienceReward`: 10 EXP
- `ActionType`: `"activity:training_complete"`

### Lifecycle & State Machine
1. **Start (`StartTraining`)**:
   - Creates an `Activity` record with `StartedAt = now`, `AvailableAt = now + 1 hour`, `Claimed = false`.
   - Enqueues a `ScheduledAction` (`activity:training_complete`) targeting `AvailableAt` into the Valkey queue.
2. **Push Resolution (Valkey Worker)**:
   - When `AvailableAt` is reached, the Worker dequeues the action, acquires a distributed lock, and executes `TrainingHandler`.
   - `TrainingHandler` applies the 10 EXP to the character and marks `Claimed = true` in a single atomic database transaction.
3. **Manual Claim Fallback (`Claim`)**:
   - The existing manual claim endpoint can be used after `AvailableAt`; production startup still requires Valkey. This is not a general runtime failover contract.
   - `ClaimAndApply` executes an atomic compare-and-set on the `claimed` column to prevent duplicate reward application under concurrent attempts.
