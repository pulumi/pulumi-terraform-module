package generated_program;

import com.pulumi.Context;
import com.pulumi.Pulumi;
import com.pulumi.core.Output;
import com.pulumi.vpc.Module;
import com.pulumi.vpc.ModuleArgs;
import java.util.List;
import java.util.ArrayList;
import java.util.Map;
import java.io.File;
import java.nio.file.Files;
import java.nio.file.Paths;

public class App {
    public static void main(String[] args) {
        Pulumi.run(App::stack);
    }

    public static void stack(Context ctx) {
        var defaultVpc = new Module("defaultVpc", ModuleArgs.builder()
            .cidr("10.0.0.0/16")
            .build());

        ctx.export("vpcId", defaultVpc.vpc_id());
    }
}
