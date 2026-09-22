import { useMutation } from "@tanstack/react-query";
import { useAuth } from "@/shared/context";

interface LoginCredentials {
  username: string;
  password: string;
}

export function useLoginSimple() {
  const { loginSimple } = useAuth();

  return useMutation({
    mutationFn: ({ username, password }: LoginCredentials) =>
      loginSimple(username, password),
  });
}
