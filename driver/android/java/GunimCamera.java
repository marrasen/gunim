package gunim.android;

import android.content.Context;
import android.content.pm.PackageManager;
import android.graphics.ImageFormat;
import android.hardware.camera2.CameraAccessException;
import android.hardware.camera2.CameraCaptureSession;
import android.hardware.camera2.CameraCharacteristics;
import android.hardware.camera2.CameraDevice;
import android.hardware.camera2.CameraManager;
import android.hardware.camera2.CaptureRequest;
import android.hardware.camera2.params.StreamConfigurationMap;
import android.media.Image;
import android.media.ImageReader;
import android.os.Handler;
import android.os.HandlerThread;
import android.util.Size;
import android.view.Display;
import android.view.Surface;
import java.util.Collections;

/**
 * GunimCamera runs the camera for Go, which reads each frame on the camera's
 * own thread, in the memory the camera gave it in. Nothing is written
 * anywhere. One runs at a time, and all of it is done on that thread but
 * start and stop.
 */
final class GunimCamera {
	// Why the camera does not run, as Go has it.
	private static final int NONE = 1, PERMISSION = 2, BUSY = 3, FAILED = 4;

	// MOST is how many pixels a frame has at the most, each way. It is
	// enough to read a code by, and more takes longer to look through.
	private static final int MOST = 1280;

	private static Handler on;

	// current is the camera that runs, or null.
	private static GunimCamera current;

	private final long run;
	private final boolean front;
	private CameraDevice device;
	private ImageReader reader;
	private int sensor;
	private boolean faces, closed;

	private GunimCamera(long run, boolean front) {
		this.run = run;
		this.front = front;
	}

	private static synchronized Handler handler() {
		if (on == null) {
			HandlerThread t = new HandlerThread("gunim-camera");
			t.start();
			on = new Handler(t.getLooper());
		}
		return on;
	}

	/** start runs the camera as run, in place of one that runs. */
	static void start(long run, boolean front) {
		handler().post(() -> {
			if (current != null) {
				current.close();
			}
			GunimCamera c = new GunimCamera(run, front);
			current = c;
			c.open();
		});
	}

	/** stop stops the camera that runs as run. */
	static void stop(long run) {
		handler().post(() -> {
			if (current != null && current.run == run) {
				current.close();
			}
		});
	}

	private void open() {
		Context app = Native.app;
		if (app == null) {
			end(FAILED);
			return;
		}
		if (app.checkSelfPermission(android.Manifest.permission.CAMERA) != PackageManager.PERMISSION_GRANTED) {
			end(PERMISSION);
			return;
		}
		CameraManager m = app.getSystemService(CameraManager.class);
		try {
			String id = null;
			for (String i : m.getCameraIdList()) {
				Integer facing = m.getCameraCharacteristics(i).get(CameraCharacteristics.LENS_FACING);
				boolean is = facing != null && facing == CameraCharacteristics.LENS_FACING_FRONT;
				if (is == front) {
					id = i;
					break;
				}
				if (id == null) {
					id = i;
				}
			}
			if (id == null) {
				end(NONE);
				return;
			}
			CameraCharacteristics k = m.getCameraCharacteristics(id);
			Integer turn = k.get(CameraCharacteristics.SENSOR_ORIENTATION), facing = k.get(CameraCharacteristics.LENS_FACING);
			sensor = turn == null ? 0 : turn;
			faces = facing != null && facing == CameraCharacteristics.LENS_FACING_FRONT;
			Size size = sizeOf(k);
			if (size == null) {
				end(FAILED);
				return;
			}
			reader = ImageReader.newInstance(size.getWidth(), size.getHeight(), ImageFormat.YUV_420_888, 3);
			reader.setOnImageAvailableListener(this::frame, on);
			boolean focuses = has(k.get(CameraCharacteristics.CONTROL_AF_AVAILABLE_MODES),
				CaptureRequest.CONTROL_AF_MODE_CONTINUOUS_PICTURE);
			m.openCamera(id, new CameraDevice.StateCallback() {
				@Override
				public void onOpened(CameraDevice d) {
					if (closed) {
						d.close();
						return;
					}
					device = d;
					session(focuses);
				}

				@Override
				public void onDisconnected(CameraDevice d) {
					d.close();
					end(BUSY);
				}

				@Override
				public void onError(CameraDevice d, int error) {
					d.close();
					end(error == ERROR_CAMERA_IN_USE || error == ERROR_MAX_CAMERAS_IN_USE || error == ERROR_CAMERA_DISABLED
						? BUSY : FAILED);
				}
			}, on);
		} catch (CameraAccessException e) {
			end(reason(e));
		} catch (SecurityException e) {
			end(PERMISSION);
		} catch (RuntimeException e) {
			end(FAILED);
		}
	}

	// session has the camera send its frames to the reader.
	@SuppressWarnings("deprecation")
	private void session(boolean focuses) {
		try {
			device.createCaptureSession(Collections.singletonList(reader.getSurface()), new CameraCaptureSession.StateCallback() {
				@Override
				public void onConfigured(CameraCaptureSession s) {
					if (closed) {
						return;
					}
					try {
						CaptureRequest.Builder b = device.createCaptureRequest(CameraDevice.TEMPLATE_PREVIEW);
						b.addTarget(reader.getSurface());
						if (focuses) {
							b.set(CaptureRequest.CONTROL_AF_MODE, CaptureRequest.CONTROL_AF_MODE_CONTINUOUS_PICTURE);
						}
						s.setRepeatingRequest(b.build(), null, on);
					} catch (CameraAccessException e) {
						end(reason(e));
					} catch (RuntimeException e) {
						end(FAILED);
					}
				}

				@Override
				public void onConfigureFailed(CameraCaptureSession s) {
					end(FAILED);
				}
			}, on);
		} catch (CameraAccessException e) {
			end(reason(e));
		} catch (RuntimeException e) {
			end(FAILED);
		}
	}

	private static int reason(CameraAccessException e) {
		switch (e.getReason()) {
		case CameraAccessException.CAMERA_IN_USE:
		case CameraAccessException.MAX_CAMERAS_IN_USE:
		case CameraAccessException.CAMERA_DISABLED:
		case CameraAccessException.CAMERA_DISCONNECTED:
			return BUSY;
		}
		return FAILED;
	}

	private static boolean has(int[] all, int one) {
		for (int i = 0; all != null && i < all.length; i++) {
			if (all[i] == one) {
				return true;
			}
		}
		return false;
	}

	// sizeOf returns the largest size of frames within MOST, or the
	// smallest there is.
	private static Size sizeOf(CameraCharacteristics k) {
		StreamConfigurationMap map = k.get(CameraCharacteristics.SCALER_STREAM_CONFIGURATION_MAP);
		Size[] all = map == null ? null : map.getOutputSizes(ImageFormat.YUV_420_888);
		if (all == null) {
			return null;
		}
		Size best = null;
		for (Size s : all) {
			if (best == null) {
				best = s;
				continue;
			}
			boolean fits = within(s), did = within(best);
			long area = area(s), was = area(best);
			if (fits != did ? fits : fits == area > was && area != was) {
				best = s;
			}
		}
		return best;
	}

	private static boolean within(Size s) {
		return s.getWidth() <= MOST && s.getHeight() <= MOST;
	}

	private static long area(Size s) {
		return (long) s.getWidth() * s.getHeight();
	}

	// frame hands Go the last frame there is.
	private void frame(ImageReader r) {
		if (closed) {
			return;
		}
		try (Image i = r.acquireLatestImage()) {
			if (i == null) {
				return;
			}
			Image.Plane[] p = i.getPlanes();
			Native.cameraFrame(run, p[0].getBuffer(), p[1].getBuffer(), p[2].getBuffer(), i.getWidth(), i.getHeight(),
				p[0].getRowStride(), p[1].getRowStride(), p[1].getPixelStride(), turn(), faces);
		} catch (RuntimeException e) {
			end(FAILED);
		}
	}

	// turn is how many degrees clockwise a frame is turned to stand upright
	// on the screen.
	private int turn() {
		GunimView v = Native.view;
		Display d = v == null ? null : v.getDisplay();
		int screen = 0;
		switch (d == null ? Surface.ROTATION_0 : d.getRotation()) {
		case Surface.ROTATION_90:
			screen = 90;
			break;
		case Surface.ROTATION_180:
			screen = 180;
			break;
		case Surface.ROTATION_270:
			screen = 270;
			break;
		}
		return (faces ? sensor + screen : sensor - screen + 360) % 360;
	}

	// end closes the camera, and tells Go why it does not run.
	private void end(int why) {
		if (closed) {
			return;
		}
		close();
		Native.cameraStopped(run, why);
	}

	private void close() {
		closed = true;
		if (current == this) {
			current = null;
		}
		if (device != null) {
			device.close();
			device = null;
		}
		if (reader != null) {
			reader.close();
			reader = null;
		}
	}
}
