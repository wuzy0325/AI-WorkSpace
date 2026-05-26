import { computed, type ComputedRef } from 'vue'
import type { StepSegment } from '@shared/types/traversal'

interface SegmentValidationError {
  index: number
  field: 'start' | 'end' | 'step'
  message: string
}

function validateSegment(
  segment: StepSegment,
  index: number,
  rangeMin: number,
  rangeMax: number,
  messages: {
    stepMustBePositive: string
    startMustBeLessThanEnd: string
    startOutOfRange: string
    endOutOfRange: string
  }
): SegmentValidationError[] {
  const errors: SegmentValidationError[] = []

  if (segment.step <= 0) {
    errors.push({ index, field: 'step', message: messages.stepMustBePositive })
  }

  if (segment.start > segment.end) {
    errors.push({ index, field: 'start', message: messages.startMustBeLessThanEnd })
  }

  if (segment.start < rangeMin || segment.start > rangeMax) {
    errors.push({ index, field: 'start', message: `${messages.startOutOfRange} (${rangeMin}~${rangeMax})` })
  }

  if (segment.end < rangeMin || segment.end > rangeMax) {
    errors.push({ index, field: 'end', message: `${messages.endOutOfRange} (${rangeMin}~${rangeMax})` })
  }

  return errors
}

export function validateSegments(
  segments: StepSegment[],
  rangeMin: number,
  rangeMax: number,
  messages: {
    stepMustBePositive: string
    startMustBeLessThanEnd: string
    startOutOfRange: string
    endOutOfRange: string
  }
): SegmentValidationError[] {
  return segments.flatMap((segment, index) => validateSegment(segment, index, rangeMin, rangeMax, messages))
}

export function getSegmentError(
  errors: SegmentValidationError[],
  index: number,
  field: 'start' | 'end' | 'step'
): string | undefined {
  return errors.find((e) => e.index === index && e.field === field)?.message
}

export function hasSegmentError(
  errors: SegmentValidationError[],
  index: number,
  field: 'start' | 'end' | 'step'
): boolean {
  return errors.some((e) => e.index === index && e.field === field)
}

export function useTraversalSegmentValidation(
  segments: ComputedRef<StepSegment[]>,
  rangeMin: ComputedRef<number>,
  rangeMax: ComputedRef<number>,
  t: ComputedRef<Record<string, string>>
) {
  const messages = computed(() => ({
    stepMustBePositive: t.value.stepMustBePositive || 'Step must be positive',
    startMustBeLessThanEnd: t.value.startMustBeLessThanEnd || 'Start must be less than end',
    startOutOfRange: t.value.startOutOfRange || 'Start out of range',
    endOutOfRange: t.value.endOutOfRange || 'End out of range'
  }))

  const errors = computed(() =>
    validateSegments(segments.value, rangeMin.value, rangeMax.value, messages.value)
  )

  const countError = computed(() =>
    segments.value.length === 0 ? (t.value.atLeastOneSegment || 'At least one segment required') : ''
  )

  return { errors, countError, getSegmentError, hasSegmentError }
}

