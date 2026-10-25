"""Run a small synthetic Keras training job for Monitor testing."""

import tensorflow as tf


def train(epochs: int = 1_000_000) -> None:
    """Train a small neural network and print progress after every epoch.

    Args:
        epochs: Number of complete training epochs to run.
    """
    tf.keras.utils.set_random_seed(42)
    device = "/GPU:0" if tf.config.list_physical_devices("GPU") else "/CPU:0"

    with tf.device(device):
        features = tf.random.normal((512, 16))
        targets = (
            features[:, :1] * 2.0
            - features[:, 1:2] * 0.5
            + tf.sin(features[:, 2:3])
        )

        model = tf.keras.Sequential(
            [
                tf.keras.layers.Input(shape=(16,)),
                tf.keras.layers.Dense(64, activation="relu"),
                tf.keras.layers.Dense(1),
            ]
        )
        optimizer = tf.keras.optimizers.Adam(learning_rate=0.001)
        loss_function = tf.keras.losses.MeanSquaredError()

        print(f"Training on {device} for {epochs} epochs", flush=True)
        for epoch in range(1, epochs + 1):
            with tf.GradientTape() as tape:
                predictions = model(features, training=True)
                loss = loss_function(targets, predictions)

            gradients = tape.gradient(loss, model.trainable_variables)
            optimizer.apply_gradients(zip(gradients, model.trainable_variables))
            print(f"Epoch {epoch}/{epochs} - loss: {loss.numpy():.6f}", flush=True)


if __name__ == "__main__":
    train()
