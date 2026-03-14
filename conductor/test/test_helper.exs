# Stop the application so tests can manage their own Store instances
Application.stop(:conductor)
ExUnit.start()
