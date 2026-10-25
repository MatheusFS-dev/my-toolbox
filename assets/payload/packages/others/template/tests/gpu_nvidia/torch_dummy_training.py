"""Run a small synthetic PyTorch training job for Monitor testing."""

import torch
from torch import nn


def train(epochs: int = 1_000_000) -> None:
    """Train a small neural network and print progress after every epoch.

    Args:
        epochs: Number of complete training epochs to run.
    """
    torch.manual_seed(42)
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")

    features = torch.randn(512, 16, device=device)
    targets = (
        features[:, :1] * 2.0
        - features[:, 1:2] * 0.5
        + torch.sin(features[:, 2:3])
    )

    model = nn.Sequential(
        nn.Linear(16, 64),
        nn.ReLU(),
        nn.Linear(64, 1),
    ).to(device)
    optimizer = torch.optim.Adam(model.parameters(), lr=0.001)
    loss_function = nn.MSELoss()

    print(f"Training on {device} for {epochs} epochs", flush=True)
    for epoch in range(1, epochs + 1):
        optimizer.zero_grad()
        predictions = model(features)
        loss = loss_function(predictions, targets)
        loss.backward()
        optimizer.step()
        print(f"Epoch {epoch}/{epochs} - loss: {loss.item():.6f}", flush=True)


if __name__ == "__main__":
    train()
