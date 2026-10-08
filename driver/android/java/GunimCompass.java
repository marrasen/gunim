package gunim.android;

import android.content.Context;
import android.hardware.Sensor;
import android.hardware.SensorEvent;
import android.hardware.SensorEventListener;
import android.hardware.SensorManager;
import android.view.Display;

/**
 * GunimCompass runs the sensors that tell which way the phone faces, while
 * Go watches the heading, and hands Go the phone's rotation matrix at each
 * reading, with the screen's rotation; Go works the heading out from them.
 * The rotation vector, which steadies the magnetometer with the gyroscope,
 * serves where the phone has one, and the accelerometer and the
 * magnetometer where it has not. None of them needs a permission.
 */
final class GunimCompass implements SensorEventListener {
	private static GunimCompass the;

	private final SensorManager sensors;
	private final Sensor rotation, gravity, field;
	// r is the rotation matrix, and q the rotation vector cut to the four
	// values every Android takes. g and m are the last readings of the
	// accelerometer, steadied, and of the magnetometer, for a phone with
	// no rotation vector.
	private final float[] r = new float[9], q = new float[4], g = new float[3], m = new float[3];
	private boolean on, haveG;
	// fieldAccuracy is the magnetometer's accuracy, which says whether it
	// wants calibrating, or -1 before it has said.
	private int fieldAccuracy = -1;

	private GunimCompass(Context app) {
		sensors = (SensorManager) app.getSystemService(Context.SENSOR_SERVICE);
		rotation = sensors == null ? null : sensors.getDefaultSensor(Sensor.TYPE_ROTATION_VECTOR);
		gravity = sensors == null ? null : sensors.getDefaultSensor(Sensor.TYPE_ACCELEROMETER);
		field = sensors == null ? null : sensors.getDefaultSensor(Sensor.TYPE_MAGNETIC_FIELD);
	}

	/** of returns the phone's compass. */
	static synchronized GunimCompass of(Context app) {
		if (the == null) {
			the = new GunimCompass(app);
		}
		return the;
	}

	/** has reports whether the phone has the sensors. */
	boolean has() {
		return rotation != null || gravity != null && field != null;
	}

	/**
	 * watch starts the sensors, at the rate meant for a user interface, or
	 * stops them. It runs on the UI thread, where they report.
	 */
	void watch(boolean on) {
		if (on == this.on || !has()) {
			return;
		}
		this.on = on;
		if (!on) {
			sensors.unregisterListener(this);
			haveG = false;
			fieldAccuracy = -1;
			return;
		}
		if (rotation != null) {
			sensors.registerListener(this, rotation, SensorManager.SENSOR_DELAY_UI);
			if (field != null) {
				// Only for its accuracy, which the rotation vector's leaves out on some phones.
				sensors.registerListener(this, field, SensorManager.SENSOR_DELAY_NORMAL);
			}
			return;
		}
		sensors.registerListener(this, gravity, SensorManager.SENSOR_DELAY_UI);
		sensors.registerListener(this, field, SensorManager.SENSOR_DELAY_UI);
	}

	@Override
	public void onSensorChanged(SensorEvent e) {
		switch (e.sensor.getType()) {
		case Sensor.TYPE_ROTATION_VECTOR:
			float[] v = e.values;
			if (v.length > 4) {
				// Some Androids take no more than four values.
				System.arraycopy(v, 0, q, 0, 4);
				v = q;
			}
			SensorManager.getRotationMatrixFromVector(r, v);
			// The fifth value is how far off the heading may be, in
			// radians, where the phone says.
			float error = e.values.length > 4 ? e.values[4] : -1;
			int accuracy = e.accuracy;
			if (fieldAccuracy >= 0) {
				accuracy = Math.min(accuracy, fieldAccuracy);
			}
			send(accuracy, error);
			break;
		case Sensor.TYPE_ACCELEROMETER:
			// Steadied, the accelerometer tells gravity from the hand's shakes.
			for (int i = 0; i < 3; i++) {
				g[i] = haveG ? 0.8f * g[i] + 0.2f * e.values[i] : e.values[i];
			}
			haveG = true;
			break;
		case Sensor.TYPE_MAGNETIC_FIELD:
			fieldAccuracy = e.accuracy;
			if (rotation != null) {
				break;
			}
			System.arraycopy(e.values, 0, m, 0, 3);
			if (haveG && SensorManager.getRotationMatrix(r, null, g, m)) {
				send(e.accuracy, -1);
			}
			break;
		default:
			break;
		}
	}

	@Override
	public void onAccuracyChanged(Sensor s, int accuracy) {
		if (s.getType() == Sensor.TYPE_MAGNETIC_FIELD) {
			fieldAccuracy = accuracy;
		}
	}

	/** send hands Go the rotation matrix with the screen's rotation. */
	private void send(int accuracy, float error) {
		GunimView v = Native.view;
		Display d = v != null ? v.getDisplay() : null;
		Native.heading(r, d != null ? d.getRotation() : 0, accuracy, error);
	}
}
