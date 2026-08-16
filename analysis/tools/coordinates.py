"""Coordinate conversions for the solver's top-left grid."""

from __future__ import annotations


def unity_local_to_grid(x: float, y: float) -> list[int]:
    """Convert Unity's right/up local axes to the solver's down/right axes."""
    return [round(-y), round(x)]
