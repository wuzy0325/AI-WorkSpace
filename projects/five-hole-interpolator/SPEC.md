# Spec: Five-Hole Interpolator Desktop Application

## Objective

Provide a standalone desktop application for five-hole probe PRB interpolation.
The app can be built and released independently from Wind-DAQ while sharing the
same interpolation algorithm package.

## Architecture

- App shell: `projects/five-hole-interpolator/apps/desktop-wails`
- Shared algorithm: `shared/algorithms/go/fivehole`
- Wind-DAQ integration: `projects/wind-daq` imports the shared algorithm module

## Success Criteria

- The app builds without importing `wind-daq/services/api-go`.
- Shared interpolation tests pass in `shared/algorithms/go/fivehole`.
- Wind-DAQ backend tests continue to pass against the shared algorithm.
- Wails app backend tests pass from the independent project path.
